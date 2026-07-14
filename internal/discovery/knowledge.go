package discovery

// Technology defines static facts and targets related to a web framework/technology.
type Technology struct {
	ID                     string
	DisplayName            string
	InterestingFiles       []string
	InterestingDirectories []string
	DiscoveryThreshold     float32
	PriorityWeights        map[string]int
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
				DiscoveryThreshold:     0.8,
				PriorityWeights: map[string]int{
					".env":      100,
					"artisan":   80,
					"storage":   50,
					"bootstrap": 50,
				},
			},
			"wordpress": {
				ID:                     "wordpress",
				DisplayName:            "WordPress",
				InterestingFiles:       []string{"xmlrpc.php", "wp-login.php"},
				InterestingDirectories: []string{"wp-admin", "wp-content", "wp-includes"},
				DiscoveryThreshold:     0.8,
				PriorityWeights: map[string]int{
					"wp-login.php": 100,
					"xmlrpc.php":   60,
					"wp-admin":     80,
					"wp-content":   50,
				},
			},
		},
	}
}

// Lookup returns the static Technology definition for a given ID.
func (r *Registry) Lookup(id string) (Technology, bool) {
	t, ok := r.techs[id]
	return t, ok
}
