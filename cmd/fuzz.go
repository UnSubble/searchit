package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/unsubble/searchit/internal/app"
	"github.com/unsubble/searchit/internal/config"
	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/filter"
	"github.com/unsubble/searchit/internal/fuzz"
	"github.com/unsubble/searchit/internal/output"
	"github.com/unsubble/searchit/internal/size"
	"github.com/unsubble/searchit/internal/stats"
	"github.com/unsubble/searchit/internal/status"
	"github.com/unsubble/searchit/internal/requesttemplate"
	"github.com/unsubble/searchit/internal/wordlist"
	"golang.org/x/time/rate"
	"regexp"
)

var (
	flagFuzzURL         string
	flagFuzzWordlist    string
	flagFuzzFoo         string
	flagFuzzBar         string
	flagFuzzBuzz        string
	flagFuzzMethod      string
	flagFuzzData        string
	flagFuzzHeaders     []string
	flagFuzzThreads     int
	flagFuzzTimeout     int
	flagFuzzExcludeStat string
	flagFuzzIncSize     string
	flagFuzzExcSize     string
	flagFuzzOutput      string
	flagFuzzFormat      string
	flagFuzzQuiet       bool
	flagFuzzDelay       string
	flagFuzzRate        float64
	flagFuzzCookie      string
	flagFuzzProxy       string

	flagFuzzMatchStatus   string
	flagFuzzFilterStatus  string
	flagFuzzMatchSize     string
	flagFuzzFilterSize    string
	flagFuzzMatchRegex    []string
	flagFuzzFilterRegex   []string
	flagFuzzMatchContent  []string
	flagFuzzFilterContent []string
	flagFuzzShowHeaders   bool
	flagFuzzShowTitle     bool
	flagFuzzRequestFile   string
)

var fuzzCmd = &cobra.Command{
	Use:   "fuzz",
	Short: "Fuzz parameters, paths, subdomains and bodies",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if flagFuzzURL == "" && flagFuzzRequestFile == "" {
			return fmt.Errorf("target URL is required (use -u or --url) or request template (--request)")
		}
		if flagFuzzThreads < 1 {
			return fmt.Errorf("threads must be at least 1")
		}

		// Ensure that at least one placeholder wordlist is provided
		hasPlaceholder := strings.Contains(flagFuzzURL, "FUZZ") ||
			strings.Contains(flagFuzzURL, "FOO") ||
			strings.Contains(flagFuzzURL, "BAR") ||
			strings.Contains(flagFuzzURL, "BUZZ") ||
			strings.Contains(flagFuzzData, "FUZZ") ||
			strings.Contains(flagFuzzData, "FOO") ||
			strings.Contains(flagFuzzData, "BAR") ||
			strings.Contains(flagFuzzData, "BUZZ")

		for _, h := range flagFuzzHeaders {
			if strings.Contains(h, "FUZZ") || strings.Contains(h, "FOO") || strings.Contains(h, "BAR") || strings.Contains(h, "BUZZ") {
				hasPlaceholder = true
			}
		}

		if flagFuzzRequestFile != "" {
			content, err := os.ReadFile(flagFuzzRequestFile)
			if err != nil {
				return fmt.Errorf("failed to read request template file: %w", err)
			}
			contentStr := string(content)
			if strings.Contains(contentStr, "FUZZ") ||
				strings.Contains(contentStr, "FOO") ||
				strings.Contains(contentStr, "BAR") ||
				strings.Contains(contentStr, "BUZZ") {
				hasPlaceholder = true
			}
		}

		if !hasPlaceholder {
			return fmt.Errorf("no placeholders (FUZZ, FOO, BAR, BUZZ) found in URL, body or headers")
		}

		usesFUZZ := strings.Contains(flagFuzzURL, "FUZZ") || strings.Contains(flagFuzzData, "FUZZ")
		usesFOO := strings.Contains(flagFuzzURL, "FOO") || strings.Contains(flagFuzzData, "FOO")
		usesBAR := strings.Contains(flagFuzzURL, "BAR") || strings.Contains(flagFuzzData, "BAR")
		usesBUZZ := strings.Contains(flagFuzzURL, "BUZZ") || strings.Contains(flagFuzzData, "BUZZ")

		if flagFuzzRequestFile != "" {
			content, _ := os.ReadFile(flagFuzzRequestFile)
			contentStr := string(content)
			if strings.Contains(contentStr, "FUZZ") {
				usesFUZZ = true
			}
			if strings.Contains(contentStr, "FOO") {
				usesFOO = true
			}
			if strings.Contains(contentStr, "BAR") {
				usesBAR = true
			}
			if strings.Contains(contentStr, "BUZZ") {
				usesBUZZ = true
			}
		}

		if usesFUZZ {
			if flagFuzzWordlist == "" {
				return fmt.Errorf("placeholder FUZZ is used but no primary wordlist provided (use -w or --wordlist)")
			}
		}

		if usesFOO {
			if flagFuzzFoo == "" {
				return fmt.Errorf("placeholder FOO is used but no --foo wordlist provided")
			}
		}

		if usesBAR {
			if flagFuzzBar == "" {
				return fmt.Errorf("placeholder BAR is used but no --bar wordlist provided")
			}
		}

		if usesBUZZ {
			if flagFuzzBuzz == "" {
				return fmt.Errorf("placeholder BUZZ is used but no --buzz wordlist provided")
			}
		}

		if flagFuzzExcludeStat != "" {
			if _, err := status.Parse(flagFuzzExcludeStat); err != nil {
				return fmt.Errorf("invalid exclude-status: %w", err)
			}
		}

		if flagFuzzIncSize != "" {
			if _, err := size.Parse(flagFuzzIncSize); err != nil {
				return fmt.Errorf("invalid include-size: %w", err)
			}
		}
		if flagFuzzExcSize != "" {
			if _, err := size.Parse(flagFuzzExcSize); err != nil {
				return fmt.Errorf("invalid exclude-size: %w", err)
			}
		}

		if flagFuzzDelay != "" {
			if _, err := time.ParseDuration(flagFuzzDelay); err != nil {
				return fmt.Errorf("invalid delay: %w", err)
			}
		}

		if cmd.Flags().Changed("rate") && flagFuzzRate <= 0 {
			return fmt.Errorf("rate must be greater than 0")
		}

		if flagFuzzOutput != "" {
			if fi, err := os.Stat(flagFuzzOutput); err == nil && fi.IsDir() {
				return fmt.Errorf("--output %q is a directory; provide a file path", flagFuzzOutput)
			}
		}

		if cmd.Flags().Changed("format") {
			if _, err := output.Parse(flagFuzzFormat); err != nil {
				return fmt.Errorf("invalid --format: %w", err)
			}
		}

		if flagFuzzProxy != "" {
			if _, err := url.Parse(flagFuzzProxy); err != nil {
				return fmt.Errorf("invalid proxy URL %q: %w", flagFuzzProxy, err)
			}
		}

		if flagFuzzMatchStatus != "" {
			if _, err := status.Parse(flagFuzzMatchStatus); err != nil {
				return fmt.Errorf("invalid match-status (--mc): %w", err)
			}
		}
		if flagFuzzFilterStatus != "" {
			if _, err := status.Parse(flagFuzzFilterStatus); err != nil {
				return fmt.Errorf("invalid filter-status (--fc): %w", err)
			}
		}
		if flagFuzzMatchSize != "" {
			if _, err := size.Parse(flagFuzzMatchSize); err != nil {
				return fmt.Errorf("invalid match-size (--ms): %w", err)
			}
		}
		if flagFuzzFilterSize != "" {
			if _, err := size.Parse(flagFuzzFilterSize); err != nil {
				return fmt.Errorf("invalid filter-size (--fs): %w", err)
			}
		}
		for _, rx := range flagFuzzMatchRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid match-regex (--mr) %q: %w", rx, err)
			}
		}
		for _, rx := range flagFuzzFilterRegex {
			if _, err := regexp.Compile(rx); err != nil {
				return fmt.Errorf("invalid filter-regex (--fr) %q: %w", rx, err)
			}
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		stats.GlobalInstrumentation.Reset()
		atomic.StoreInt32(&stats.GlobalInstrumentation.Enabled, 1)

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// Initialize default config and HTTP client options
		cfg := config.Default()
		cfg.Threads = flagFuzzThreads
		cfg.Timeout = time.Duration(flagFuzzTimeout) * time.Second
		cfg.Quiet = flagFuzzQuiet
		cfg.Proxy = flagFuzzProxy
		cfg.ShowHeaders = flagFuzzShowHeaders
		cfg.ShowTitle = flagFuzzShowTitle

		if flagFuzzMatchStatus != "" {
			exclude, _ := status.Parse(flagFuzzMatchStatus)
			cfg.Status.Include = exclude
		}
		if flagFuzzFilterStatus != "" {
			exclude, _ := status.Parse(flagFuzzFilterStatus)
			cfg.Status.Exclude = exclude
		} else if flagFuzzExcludeStat != "" {
			exclude, _ := status.Parse(flagFuzzExcludeStat)
			cfg.Status.Exclude = exclude
		}

		if flagFuzzMatchSize != "" {
			inc, _ := size.Parse(flagFuzzMatchSize)
			cfg.IncludeSize = inc
		} else if flagFuzzIncSize != "" {
			inc, _ := size.Parse(flagFuzzIncSize)
			cfg.IncludeSize = inc
		}
		if flagFuzzFilterSize != "" {
			exc, _ := size.Parse(flagFuzzFilterSize)
			cfg.ExcludeSize = exc
		} else if flagFuzzExcSize != "" {
			exc, _ := size.Parse(flagFuzzExcSize)
			cfg.ExcludeSize = exc
		}
		cfg.MatchRegex = flagFuzzMatchRegex
		cfg.FilterRegex = flagFuzzFilterRegex
		cfg.MatchContent = flagFuzzMatchContent
		cfg.FilterContent = flagFuzzFilterContent

		var delay time.Duration
		if flagFuzzDelay != "" {
			delay, _ = time.ParseDuration(flagFuzzDelay)
		}

		var limiter *rate.Limiter
		if flagFuzzRate > 0 {
			limiter = rate.NewLimiter(rate.Limit(flagFuzzRate), 1)
		}

		// Load auxiliary wordlists
		fooWords, err := loadLines(flagFuzzFoo)
		if err != nil {
			return fmt.Errorf("failed to load FOO wordlist: %w", err)
		}
		barWords, err := loadLines(flagFuzzBar)
		if err != nil {
			return fmt.Errorf("failed to load BAR wordlist: %w", err)
		}
		buzzWords, err := loadLines(flagFuzzBuzz)
		if err != nil {
			return fmt.Errorf("failed to load BUZZ wordlist: %w", err)
		}

		var headers http.Header
		if flagFuzzRequestFile != "" {
			tmpl, err := requesttemplate.ParseFile(flagFuzzRequestFile, flagFuzzURL)
			if err != nil {
				return err
			}
			flagFuzzURL = tmpl.URL
			flagFuzzMethod = tmpl.Method
			flagFuzzData = string(tmpl.Body)

			headers = make(http.Header)
			for k, values := range tmpl.Headers {
				for _, v := range values {
					headers.Add(k, v)
				}
			}

			cliHeaders, err := parseFuzzHeaderFlags(flagFuzzHeaders)
			if err != nil {
				return err
			}
			for k, values := range cliHeaders {
				for _, v := range values {
					headers.Add(k, v)
				}
			}

			cookies := requesttemplate.ExtractCookiesFromHeaders(tmpl.Headers)
			if len(cookies) > 0 {
				var cookiePairs []string
				for _, c := range cookies {
					cookiePairs = append(cookiePairs, c.String())
				}
				flagFuzzCookie = strings.Join(cookiePairs, "; ")
			}
		} else {
			cliHeaders, err := parseFuzzHeaderFlags(flagFuzzHeaders)
			if err != nil {
				return err
			}
			headers = cliHeaders
		}

		// Resolve output format
		outFormat := output.FormatText
		if flagFuzzOutput != "" {
			outFormat = output.FormatFromPath(flagFuzzOutput)
		}
		if cmd.Flags().Changed("format") {
			parsedFormat, _ := output.Parse(flagFuzzFormat)
			outFormat = parsedFormat
		}

		outWriter := os.Stdout
		if flagFuzzOutput != "" {
			f, err := os.OpenFile(flagFuzzOutput, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return fmt.Errorf("cannot open output file: %w", err)
			}
			defer f.Close()
			outWriter = f
		}

		fmttr := output.New(outFormat, outWriter, cfg.Quiet, cfg.ShowHeaders, cfg.ShowTitle)
		defer fmttr.Close()

		if outFormat == output.FormatText && !cfg.Quiet {
			fmt.Printf("[*] Fuzzing target: %s\n", flagFuzzURL)
			if flagFuzzWordlist != "" {
				fmt.Printf("[*] Primary wordlist: %s\n", flagFuzzWordlist)
			}
		}

		// Setup the primary wordlist reader if provided
		var reader wordlist.Reader
		var primaryChan chan string
		if flagFuzzWordlist != "" {
			reader = wordlist.FileReader{Path: flagFuzzWordlist}
			primaryChan = make(chan string, 100)
			go func() {
				defer close(primaryChan)
				_ = reader.Read(ctx, primaryChan)
			}()
		}

		appState := app.New(ctx, cfg)

		fs, err := filter.NewFilterSuite(
			cfg.Status.Include.String(),
			cfg.Status.Exclude.String(),
			cfg.IncludeSize.String(),
			cfg.ExcludeSize.String(),
			cfg.MatchRegex,
			cfg.FilterRegex,
			cfg.MatchContent,
			cfg.FilterContent,
		)
		if err != nil {
			return err
		}
		fs.ShowHeaders = cfg.ShowHeaders
		fs.ShowTitle = cfg.ShowTitle

		// Start generator
		generator := fuzz.NewGenerator(
			flagFuzzURL,
			strings.ToUpper(flagFuzzMethod),
			flagFuzzData,
			headers,
			flagFuzzCookie,
			fooWords,
			barWords,
			buzzWords,
		)

		jobs := make(chan fuzz.Job, cfg.Threads)
		go func() {
			defer close(jobs)
			generator.Generate(ctx, primaryChan, jobs)
		}()

		// Start concurrency pool
		collector := stats.NewCollector()
		results := fuzz.Start(
			ctx,
			appState.HTTPClient,
			fs,
			cfg.Threads,
			delay,
			limiter,
			jobs,
			collector,
		)

		// Stream results to output formatter
		for r := range results {
			if r.Accepted {
				engRes := engine.Result{
					URL:        r.URL,
					StatusCode: r.StatusCode,
					Length:     r.Length,
					Accepted:   r.Accepted,
					Err:        r.Err,
					Origin:     "fuzz",
					Title:      r.Title,
					Headers:    r.Headers,
				}
				_ = fmttr.Print(engRes)
			}
		}

		return nil
	},
}

func loadLines(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, scanner.Err()
}

func parseFuzzHeaderFlags(flags []string) (http.Header, error) {
	headers := make(http.Header)
	for _, h := range flags {
		idx := strings.Index(h, ":")
		if idx == -1 {
			idx = strings.Index(h, "=")
		}
		if idx <= 0 || idx == len(h)-1 {
			return nil, fmt.Errorf("header flag %q must be in Name: Value or Name=Value format", h)
		}
		name := strings.TrimSpace(h[:idx])
		val := strings.TrimSpace(h[idx+1:])
		headers.Add(name, val)
	}
	return headers, nil
}

func init() {
	rootCmd.AddCommand(fuzzCmd)

	fuzzCmd.Flags().StringVarP(&flagFuzzURL, "url", "u", "", "target URL with placeholders (FUZZ, FOO, BAR, BUZZ)")
	fuzzCmd.Flags().StringVarP(&flagFuzzWordlist, "wordlist", "w", "", "primary wordlist path (maps to FUZZ)")
	fuzzCmd.Flags().StringVar(&flagFuzzFoo, "foo", "", "wordlist path for FOO placeholder")
	fuzzCmd.Flags().StringVar(&flagFuzzBar, "bar", "", "wordlist path for BAR placeholder")
	fuzzCmd.Flags().StringVar(&flagFuzzBuzz, "buzz", "", "wordlist path for BUZZ placeholder")
	fuzzCmd.Flags().StringVarP(&flagFuzzMethod, "method", "X", "GET", "HTTP method")
	fuzzCmd.Flags().StringVarP(&flagFuzzData, "data", "d", "", "POST request data body with placeholders")
	fuzzCmd.Flags().StringSliceVarP(&flagFuzzHeaders, "header", "H", nil, "custom request headers with placeholders (e.g. -H 'X-Header=FUZZ')")
	fuzzCmd.Flags().IntVarP(&flagFuzzThreads, "threads", "t", 32, "number of concurrent worker threads")
	fuzzCmd.Flags().IntVar(&flagFuzzTimeout, "timeout", 10, "request timeout in seconds")
	fuzzCmd.Flags().StringVarP(&flagFuzzExcludeStat, "exclude-status", "x", "404", "comma-separated status codes to exclude")
	fuzzCmd.Flags().StringVar(&flagFuzzIncSize, "include-size", "", "comma-separated exact sizes or ranges to include")
	fuzzCmd.Flags().StringVar(&flagFuzzExcSize, "exclude-size", "", "comma-separated exact sizes or ranges to exclude")
	fuzzCmd.Flags().StringVarP(&flagFuzzOutput, "output", "o", "", "write results to this file (default: stdout)")
	fuzzCmd.Flags().StringVar(&flagFuzzFormat, "format", "text", "explicit output format (text, json, ndjson, csv, markdown)")
	fuzzCmd.Flags().BoolVarP(&flagFuzzQuiet, "quiet", "q", false, "disable status prefix printing in stdout")
	fuzzCmd.Flags().StringVar(&flagFuzzDelay, "delay", "", "delay between requests (e.g. 50ms, 1s)")
	fuzzCmd.Flags().Float64Var(&flagFuzzRate, "rate", 0, "maximum requests per second rate limit")
	fuzzCmd.Flags().StringVarP(&flagFuzzCookie, "cookie", "b", "", "HTTP request cookies with placeholders")
	fuzzCmd.Flags().StringVar(&flagFuzzProxy, "proxy", "", "HTTP proxy URL to use for requests")

	fuzzCmd.Flags().StringVar(&flagFuzzMatchStatus, "mc", "", "match status codes")
	fuzzCmd.Flags().StringVar(&flagFuzzFilterStatus, "fc", "", "filter status codes")
	fuzzCmd.Flags().StringVar(&flagFuzzMatchSize, "ms", "", "match size")
	fuzzCmd.Flags().StringVar(&flagFuzzFilterSize, "fs", "", "filter size")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzMatchRegex, "mr", nil, "match regex")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzFilterRegex, "fr", nil, "filter regex")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzMatchContent, "mt", nil, "match content types")
	fuzzCmd.Flags().StringSliceVar(&flagFuzzFilterContent, "ft", nil, "filter content types")
	fuzzCmd.Flags().BoolVar(&flagFuzzShowHeaders, "show-headers", false, "show response headers in output")
	fuzzCmd.Flags().BoolVar(&flagFuzzShowTitle, "show-title", false, "show HTML titles in output")
	fuzzCmd.Flags().StringVar(&flagFuzzRequestFile, "request", "", "load raw HTTP request template from file")
}
