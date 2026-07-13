package adaptive

import (
	"sort"
	"strings"
)

// builtins is the authoritative list of supported technology IDs.
// Keys are canonical lowercase IDs; values are their display names.
// Add new technologies here — no other file needs to change.
var builtins = map[string]string{
	"angular":   "Angular",
	"aspnet":    "ASP.NET",
	"django":    "Django",
	"express":   "Express",
	"flask":     "Flask",
	"go":        "Go",
	"laravel":   "Laravel",
	"nextjs":    "Next.js",
	"nuxt":      "Nuxt",
	"react":     "React",
	"spring":    "Spring Boot",
	"vue":       "Vue",
	"wordpress": "WordPress",
}

// Lookup returns the TechProfile for the given technology identifier.
// The lookup is case-insensitive. The second return value is false when
// the identifier does not match any built-in profile.
func Lookup(id string) (TechProfile, bool) {
	key := strings.ToLower(strings.TrimSpace(id))
	name, ok := builtins[key]
	if !ok {
		return TechProfile{}, false
	}
	return TechProfile{ID: key, DisplayName: name}, true
}

// All returns a sorted slice of all built-in TechProfiles.
// The slice is ordered alphabetically by ID.
func All() []TechProfile {
	profiles := make([]TechProfile, 0, len(builtins))
	for id, name := range builtins {
		profiles = append(profiles, TechProfile{ID: id, DisplayName: name})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].ID < profiles[j].ID
	})
	return profiles
}

// SupportedIDs returns a comma-separated list of all built-in technology IDs.
// Intended for use in error messages.
func SupportedIDs() string {
	all := All()
	ids := make([]string, len(all))
	for i, p := range all {
		ids[i] = p.ID
	}
	return strings.Join(ids, ", ")
}
