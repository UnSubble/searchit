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

// New returns an *http.Client tuned for high-concurrency scanning.
func New(timeout time.Duration, connectTimeout time.Duration, followRedirects bool, proxyURL string) *http.Client {
	return NewWithMaxRedirects(timeout, connectTimeout, followRedirects, 10, proxyURL)
}

// NewWithMaxRedirects returns an *http.Client with specified redirect limit and default 'auto' HTTP version.
func NewWithMaxRedirects(timeout time.Duration, connectTimeout time.Duration, followRedirects bool, maxRedirects int, proxyURL string) *http.Client {
	return NewWithHTTPVersion(timeout, connectTimeout, followRedirects, maxRedirects, proxyURL, "auto")
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

// NewWithHTTPVersion creates an *http.Client configured for a specific HTTP protocol version.
func NewWithHTTPVersion(
	timeout time.Duration,
	connectTimeout time.Duration,
	followRedirects bool,
	maxRedirects int,
	proxyURL string,
	httpVersion string,
) *http.Client {
	if err := ValidateHTTPVersion(httpVersion); err != nil {
		panic(err)
	}

	tr := &http.Transport{
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: timeout,
		DisableCompression:    false,
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	if proxyURL != "" {
		pURL, err := url.Parse(proxyURL)
		if err != nil {
			panic(fmt.Sprintf("invalid proxy URL %q: %v", proxyURL, err))
		}
		tr.Proxy = http.ProxyURL(pURL)
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}

	version := strings.ToLower(strings.TrimSpace(httpVersion))
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
	if !followRedirects {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		checkRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
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
		Timeout:       timeout,
		CheckRedirect: checkRedirect,
	}
}

func (p *protoTransport) Unwrap() http.RoundTripper {
	return p.tr
}

func getUnderlyingTransport(rt http.RoundTripper) *http.Transport {
	for rt != nil {
		if tr, ok := rt.(*http.Transport); ok {
			return tr
		}
		if u, ok := rt.(interface{ Unwrap() http.RoundTripper }); ok {
			rt = u.Unwrap()
		} else {
			break
		}
	}
	return nil
}

// ConfigureTransportForWorkers dynamically adjusts transport idle connection caps
// based on the configured worker thread count: max(8, workers * 2).
func ConfigureTransportForWorkers(client *http.Client, workers int) {
	if client == nil {
		return
	}
	tr := getUnderlyingTransport(client.Transport)
	if tr == nil {
		return
	}
	maxHost := workers * 2
	if maxHost < 8 {
		maxHost = 8
	}
	tr.MaxIdleConnsPerHost = maxHost
	if tr.MaxIdleConns < maxHost*10 {
		tr.MaxIdleConns = maxHost * 10
	}
}
