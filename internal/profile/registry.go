package profile

import (
	"fmt"
	"sync"
)

var (
	decodersMu sync.RWMutex
	decoders   = make(map[int]Decoder)
)

// RegisterDecoder registers a profile decoder for a specific schema version.
// Returns an error if a decoder for the same schema version is already registered.
func RegisterDecoder(d Decoder) error {
	decodersMu.Lock()
	defer decodersMu.Unlock()
	schema := d.Schema()
	if _, ok := decoders[schema]; ok {
		return fmt.Errorf("decoder for schema version %d is already registered", schema)
	}
	decoders[schema] = d
	return nil
}

// GetDecoder returns the registered decoder for the given schema version,
// or an error if the schema version is unsupported/unregistered.
func GetDecoder(schema int) (Decoder, error) {
	decodersMu.RLock()
	defer decodersMu.RUnlock()
	d, ok := decoders[schema]
	if !ok {
		return nil, fmt.Errorf("unsupported profile schema version: %d", schema)
	}
	return d, nil
}
