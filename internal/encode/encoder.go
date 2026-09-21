package encode

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Encoder transforms a string at either candidate level or wordlist level.
type Encoder interface {
	Encode(value string) string
	Name() string
	IsWordlistScoped() bool
}

// SupportedEncodings lists the supported encoding names in canonical order.
var SupportedEncodings = []string{
	"base64",
	"url",
	"doubleurl",
	"wl-base64",
	"wl-url",
	"wl-doubleurl",
}

// NoopEncoder returns the candidate value untouched.
type NoopEncoder struct{}

func (NoopEncoder) Encode(value string) string { return value }
func (NoopEncoder) Name() string               { return "" }
func (NoopEncoder) IsWordlistScoped() bool     { return false }

// Base64Encoder encodes the candidate value using standard Base64.
type Base64Encoder struct{}

func (Base64Encoder) Encode(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
func (Base64Encoder) Name() string           { return "base64" }
func (Base64Encoder) IsWordlistScoped() bool { return false }

// URLEncoder percent-encodes the candidate value according to RFC 3986.
// Unreserved characters (ALPHA, DIGIT, "-", ".", "_", "~") are preserved.
type URLEncoder struct{}

const hexChars = "0123456789ABCDEF"

func isUnreserved(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == '.' || b == '~'
}

func encodeRFC3986(s string) string {
	needsEscape := false
	for i := 0; i < len(s); i++ {
		if !isUnreserved(s[i]) {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 6)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hexChars[c>>4])
			b.WriteByte(hexChars[c&0xF])
		}
	}
	return b.String()
}

func (URLEncoder) Encode(value string) string {
	return encodeRFC3986(value)
}
func (URLEncoder) Name() string           { return "url" }
func (URLEncoder) IsWordlistScoped() bool { return false }

// DoubleURLEncoder percent-encodes the candidate value twice.
type DoubleURLEncoder struct{}

func (DoubleURLEncoder) Encode(value string) string {
	return encodeRFC3986(encodeRFC3986(value))
}
func (DoubleURLEncoder) Name() string           { return "doubleurl" }
func (DoubleURLEncoder) IsWordlistScoped() bool { return false }

// WordlistBase64Encoder encodes raw wordlist entries using standard Base64.
type WordlistBase64Encoder struct{}

func (WordlistBase64Encoder) Encode(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
func (WordlistBase64Encoder) Name() string           { return "wl-base64" }
func (WordlistBase64Encoder) IsWordlistScoped() bool { return true }

// WordlistURLEncoder percent-encodes raw wordlist entries according to RFC 3986.
type WordlistURLEncoder struct{}

func (WordlistURLEncoder) Encode(value string) string {
	return encodeRFC3986(value)
}
func (WordlistURLEncoder) Name() string           { return "wl-url" }
func (WordlistURLEncoder) IsWordlistScoped() bool { return true }

// WordlistDoubleURLEncoder percent-encodes raw wordlist entries twice.
type WordlistDoubleURLEncoder struct{}

func (WordlistDoubleURLEncoder) Encode(value string) string {
	return encodeRFC3986(encodeRFC3986(value))
}
func (WordlistDoubleURLEncoder) Name() string           { return "wl-doubleurl" }
func (WordlistDoubleURLEncoder) IsWordlistScoped() bool { return true }

// NewEncoder returns the Encoder corresponding to the given name.
// Name comparison is case-insensitive.
// An empty name returns a NoopEncoder.
func NewEncoder(name string) (Encoder, error) {
	canonical := strings.ToLower(strings.TrimSpace(name))
	switch canonical {
	case "":
		return NoopEncoder{}, nil
	case "base64":
		return Base64Encoder{}, nil
	case "url":
		return URLEncoder{}, nil
	case "doubleurl":
		return DoubleURLEncoder{}, nil
	case "wl-base64":
		return WordlistBase64Encoder{}, nil
	case "wl-url":
		return WordlistURLEncoder{}, nil
	case "wl-doubleurl":
		return WordlistDoubleURLEncoder{}, nil
	default:
		return nil, fmt.Errorf("invalid encoding %q: supported encodings are %s", name, strings.Join(SupportedEncodings, ", "))
	}
}
