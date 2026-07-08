package profile

import (
	"fmt"

	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

// Decoder defines the interface for schema-specific profile decoders.
type Decoder = types.Decoder

// Header represents the schema version header of a profile document.
type Header struct {
	Schema int `yaml:"schema"`
}

// DecodeProfile detects the schema version, looks up the decoder, and decodes the YAML data.
func DecodeProfile(data []byte) (*Profile, error) {
	var header Header
	if err := yaml.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("detect profile schema header: %w", err)
	}

	if header.Schema == 0 {
		return nil, fmt.Errorf("missing or invalid profile schema version")
	}

	decoder, err := GetDecoder(header.Schema)
	if err != nil {
		return nil, err
	}

	return decoder.Decode(data)
}
