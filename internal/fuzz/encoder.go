package fuzz

import "github.com/unsubble/searchit/internal/encode"

// Encoder type alias for backward compatibility.
type Encoder = encode.Encoder

// SupportedEncodings lists the supported encoding names in canonical order.
var SupportedEncodings = encode.SupportedEncodings

// Concrete encoder type aliases.
type NoopEncoder = encode.NoopEncoder
type Base64Encoder = encode.Base64Encoder
type URLEncoder = encode.URLEncoder
type DoubleURLEncoder = encode.DoubleURLEncoder
type WordlistBase64Encoder = encode.WordlistBase64Encoder
type WordlistURLEncoder = encode.WordlistURLEncoder
type WordlistDoubleURLEncoder = encode.WordlistDoubleURLEncoder

// NewEncoder returns the Encoder corresponding to the given name.
func NewEncoder(name string) (Encoder, error) {
	return encode.NewEncoder(name)
}
