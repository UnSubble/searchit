package discovery

// Technology defines static facts about a framework/technology.
type Technology struct {
	ID                     string
	DisplayName            string
	InterestingFiles       []string
	InterestingDirectories []string
	InterestingExtensions  []string
	InterestingHeaders     []string
	InterestingCookies     []string
}

// Registry acts as a read-only registry of all technology definitions.
type Registry struct {
	techs map[string]Technology
}

// NewRegistry creates a new technology registry with built-in definitions.
func NewRegistry() *Registry {
	return &Registry{
		techs: map[string]Technology{
			"laravel": {
				ID:                     "laravel",
				DisplayName:            "Laravel",
				InterestingFiles:       []string{".env", "artisan"},
				InterestingDirectories: []string{"storage", "bootstrap", "vendor"},
				InterestingExtensions:  []string{".php"},
				InterestingHeaders:     []string{"X-Powered-By"},
				InterestingCookies:     []string{"laravel_session"},
			},
			"wordpress": {
				ID:                     "wordpress",
				DisplayName:            "WordPress",
				InterestingFiles:       []string{"xmlrpc.php", "wp-login.php"},
				InterestingDirectories: []string{"wp-admin", "wp-content", "wp-includes"},
				InterestingExtensions:  []string{".php"},
				InterestingHeaders:     []string{"X-Link"},
				InterestingCookies:     []string{"wordpress_logged_in"},
			},
		},
	}
}

// Lookup returns the static Technology definition for a given ID.
func (r *Registry) Lookup(id string) (Technology, bool) {
	t, ok := r.techs[id]
	return t, ok
}
