package v1

import (
	"fmt"

	"github.com/unsubble/searchit/internal/profile/types"
	"gopkg.in/yaml.v3"
)

// Decoder implements the types.Decoder interface for schema v1.
type Decoder struct{}

// New returns a new schema v1 decoder.
func New() *Decoder {
	return &Decoder{}
}

// Schema returns the schema version this decoder supports.
func (d *Decoder) Schema() int {
	return 1
}

// Decode translates raw YAML into the internal runtime Profile representation.
func (d *Decoder) Decode(data []byte) (*types.Profile, error) {
	var doc ProfileDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal v1 profile: %w", err)
	}

	return &types.Profile{
		Schema:       doc.Schema,
		Name:         doc.Name,
		Tool:         doc.Tool,
		Description:  doc.Description,
		Author:       doc.Author,
		Tags:         doc.Tags,
		Homepage:     doc.Homepage,
		License:      doc.License,
		Created:      doc.Created,
		Updated:      doc.Updated,
		Depends:      doc.Depends,
		Experimental: doc.Experimental,
		Config:       doc.Config,
	}, nil
}
