package signals

// SignalType represents an observed technology or target hint.
type SignalType string

const (
	SignalRobots    SignalType = "robots"
	SignalSitemap   SignalType = "sitemap"
	SignalLaravel   SignalType = "laravel"
	SignalWordPress SignalType = "wordpress"
	SignalExpress   SignalType = "express"
	SignalJSON      SignalType = "json"
	SignalAPI       SignalType = "api"
	SignalGraphQL   SignalType = "graphql"
	SignalAdmin     SignalType = "admin"
	SignalAsset     SignalType = "asset"
)

// ScoreMap maps each SignalType to its prioritization weight.
var ScoreMap = map[SignalType]int{
	SignalRobots:    30,
	SignalSitemap:   30,
	SignalLaravel:   40,
	SignalWordPress: 35,
	SignalExpress:   25,
	SignalJSON:      25,
	SignalAPI:       20,
	SignalGraphQL:   20,
	SignalAdmin:     20,
	SignalAsset:     15,
}
