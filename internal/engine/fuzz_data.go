package engine

type FieldLocation string

const (
	LocationHeader FieldLocation = "header"
	LocationCookie FieldLocation = "cookie"
	LocationBody   FieldLocation = "body"
	LocationJSON   FieldLocation = "json"
)

// FuzzField represents a single substituted non-URL request field.
type FuzzField struct {
	Location FieldLocation `json:"location"`
	Name     string        `json:"name,omitempty"`
	Value    string        `json:"value"`
}

// FuzzData encapsulates generic substituted request fields for fuzzing output.
type FuzzData struct {
	Fields []FuzzField `json:"fields,omitempty"`
}
