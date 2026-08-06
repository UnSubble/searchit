package httpclient

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

// ValidateHTTPVersion checks if the provided httpVersion string is valid.
func ValidateHTTPVersion(v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto", "0.9", "1.0", "1.1", "2":
		return nil
	default:
		return fmt.Errorf("invalid --http-version %q: supported values are auto, 0.9, 1.0, 1.1, 2", v)
	}
}

const (
	// DefaultIdleTimeout is the duration idle connections wait in the pool before being closed.
	DefaultIdleTimeout = 90 * time.Second
	// DefaultConnectTimeout is the timeout for establishing new TCP connections.
	DefaultConnectTimeout = 30 * time.Second
	// MinConnsPerHost guarantees a minimum pool size even for low-thread scans.
	MinConnsPerHost = 8
	// IdleConnectionMultiplier ensures the global idle pool is large enough to absorb bursty worker completion.
	IdleConnectionMultiplier = 10
	// HostConnectionMultiplier provides headroom for simultaneous connections per host.
	HostConnectionMultiplier = 2
)

// VersionAuto represents the default auto-negotiation HTTP version.
const VersionAuto = "auto"

// Options represents the configuration for the HTTP client transport.
type Options struct {
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	FollowRedirects bool
	MaxRedirects    int
	ProxyURL        string
	HTTPVersion     string
	Insecure        bool
	MaxWorkers      int
}

// applyDefaults applies sane default values for any zero-valued options.
func (o *Options) applyDefaults() {
	if o.ConnectTimeout == 0 {
		o.ConnectTimeout = DefaultConnectTimeout
	}
	if o.MaxRedirects == 0 && o.FollowRedirects {
		o.MaxRedirects = 10
	}
	if o.HTTPVersion == "" {
		o.HTTPVersion = VersionAuto
	}
	if o.MaxWorkers == 0 {
		o.MaxWorkers = 1
	}
}

type protoTransport struct {
	tr    *http.Transport
	major int
	minor int
	proto string
}

func (p *protoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	if !strings.Contains(host, ":") {
		if req.URL.Scheme == "https" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}

	ctx := req.Context()
	var conn net.Conn
	var err error

	timeout := 10 * time.Second
	if p.tr.DialContext != nil {
		conn, err = p.tr.DialContext(ctx, "tcp", host)
	} else if req.URL.Scheme == "https" {
		var tlsConfig *tls.Config
		if p.tr.TLSClientConfig != nil {
			tlsConfig = p.tr.TLSClientConfig.Clone()
		} else {
			tlsConfig = &tls.Config{}
		}
		dialer := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: timeout},
			Config:    tlsConfig,
		}
		conn, err = dialer.DialContext(ctx, "tcp", host)
	} else {
		dialer := &net.Dialer{Timeout: timeout}
		conn, err = dialer.DialContext(ctx, "tcp", host)
	}

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	if err != nil {
		return nil, err
	}

	reqURI := req.URL.RequestURI()
	if reqURI == "" {
		reqURI = "/"
	}

	if p.major == 0 && p.minor == 9 {
		// HTTP/0.9: GET /path\r\n
		_, err = fmt.Fprintf(conn, "%s %s\r\n", req.Method, reqURI)
		if err != nil {
			conn.Close()
			return nil, err
		}
	} else {
		// HTTP/1.0: GET /path HTTP/1.0\r\n
		_, err = fmt.Fprintf(conn, "%s %s %s\r\n", req.Method, reqURI, p.proto)
		if err != nil {
			conn.Close()
			return nil, err
		}
		hostHeader := req.Host
		if hostHeader == "" {
			hostHeader = req.URL.Host
		}
		fmt.Fprintf(conn, "Host: %s\r\n", hostHeader)
		for k, vv := range req.Header {
			for _, v := range vv {
				fmt.Fprintf(conn, "%s: %s\r\n", k, v)
			}
		}
		fmt.Fprintf(conn, "Connection: close\r\n\r\n")
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	resp.Body = &connCloser{ReadCloser: resp.Body, conn: conn}
	return resp, nil
}

type connCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (c *connCloser) Close() error {
	err1 := c.ReadCloser.Close()
	err2 := c.conn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// New creates an *http.Client configured for a specific HTTP protocol version.
func New(opts Options) *http.Client {
	opts.applyDefaults()

	if err := ValidateHTTPVersion(opts.HTTPVersion); err != nil {
		panic(err)
	}

	maxHost := opts.MaxWorkers * HostConnectionMultiplier
	if maxHost < MinConnsPerHost {
		maxHost = MinConnsPerHost
	}
	maxIdle := maxHost * IdleConnectionMultiplier

	tr := &http.Transport{
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   maxHost,
		MaxConnsPerHost:       maxHost,
		IdleConnTimeout:       DefaultIdleTimeout,
		ResponseHeaderTimeout: opts.Timeout,
		DisableCompression:    false,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: opts.Insecure},
		DialContext: (&net.Dialer{
			Timeout:   opts.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	if opts.ProxyURL != "" {
		pURL, err := url.Parse(opts.ProxyURL)
		if err != nil {
			panic(fmt.Sprintf("invalid proxy URL %q: %v", opts.ProxyURL, err))
		}
		tr.Proxy = http.ProxyURL(pURL)
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}

	version := strings.ToLower(strings.TrimSpace(opts.HTTPVersion))
	if version == "" {
		version = "auto"
	}

	var baseTransport http.RoundTripper = tr

	switch version {
	case "2":
		tr.ForceAttemptHTTP2 = true
		_ = http2.ConfigureTransport(tr)

	case "1.1":
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)

	case "1.0":
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
		baseTransport = &protoTransport{
			tr:    tr,
			major: 1,
			minor: 0,
			proto: "HTTP/1.0",
		}

	case "0.9":
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
		baseTransport = &protoTransport{
			tr:    tr,
			major: 0,
			minor: 9,
			proto: "HTTP/0.9",
		}

	case "auto":
		tr.ForceAttemptHTTP2 = true
		_ = http2.ConfigureTransport(tr)
	}

	var checkRedirect func(req *http.Request, via []*http.Request) error
	if !opts.FollowRedirects {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return fmt.Errorf("maximum redirect limit exceeded")
			}
			for _, prev := range via {
				if prev.URL.String() == req.URL.String() {
					return fmt.Errorf("redirect loop detected")
				}
			}
			return nil
		}
	}

	return &http.Client{
		Transport:     baseTransport,
		Timeout:       opts.Timeout,
		CheckRedirect: checkRedirect,
	}
}

func (p *protoTransport) Unwrap() http.RoundTripper {
	return p.tr
}
