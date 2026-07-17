package requesttemplate_test

import (
	"testing"

	"github.com/unsubble/searchit/internal/requesttemplate"
)

func TestParse(t *testing.T) {
	t.Run("simple get request", func(t *testing.T) {
		raw := []byte("GET /info HTTP/1.1\r\nHost: myapp.local\r\n\r\n")
		tmpl, err := requesttemplate.Parse(raw, "")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if tmpl.Method != "GET" {
			t.Errorf("expected GET, got %q", tmpl.Method)
		}
		if tmpl.URL != "http://myapp.local/info" {
			t.Errorf("expected http://myapp.local/info, got %q", tmpl.URL)
		}
		if tmpl.Headers.Get("Host") != "myapp.local" {
			t.Errorf("expected Host header, got %q", tmpl.Headers.Get("Host"))
		}
	})

	t.Run("post request with body", func(t *testing.T) {
		raw := []byte("POST /submit HTTP/1.1\nHost: test.com\nContent-Length: 15\n\nusername=admin")
		tmpl, err := requesttemplate.Parse(raw, "https://override.com")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if tmpl.Method != "POST" {
			t.Errorf("expected POST, got %q", tmpl.Method)
		}
		if tmpl.URL != "https://override.com/submit" {
			t.Errorf("expected https://override.com/submit, got %q", tmpl.URL)
		}
		if string(tmpl.Body) != "username=admin" {
			t.Errorf("expected username=admin, got %q", string(tmpl.Body))
		}
	})

	t.Run("placeholder URL resolution", func(t *testing.T) {
		raw := []byte("GET /api/FUZZ HTTP/1.1\nHost: api.local\n\n")
		tmpl, err := requesttemplate.Parse(raw, "http://api.local/v1")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if tmpl.URL != "http://api.local/v1/api/FUZZ" {
			t.Errorf("expected http://api.local/v1/api/FUZZ, got %q", tmpl.URL)
		}
	})

	t.Run("cookies extraction", func(t *testing.T) {
		raw := []byte("GET / HTTP/1.1\nHost: app.local\nCookie: sess=123; user=john\n\n")
		tmpl, err := requesttemplate.Parse(raw, "")
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		cookies := requesttemplate.ExtractCookiesFromHeaders(tmpl.Headers)
		if len(cookies) != 2 {
			t.Fatalf("expected 2 cookies, got %d", len(cookies))
		}
		if cookies[0].Name != "sess" || cookies[0].Value != "123" {
			t.Errorf("unexpected cookie 0: %+v", cookies[0])
		}
		if cookies[1].Name != "user" || cookies[1].Value != "john" {
			t.Errorf("unexpected cookie 1: %+v", cookies[1])
		}
		if tmpl.Headers.Get("Cookie") != "" {
			t.Error("Cookie header was not deleted from Headers map")
		}
	})
}
