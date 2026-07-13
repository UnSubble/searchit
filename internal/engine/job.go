package engine

// Job is the unit of work dispatched to a worker.
// One Job produces at most one Result.
type Job struct {
	URL    string
	Depth  uint16
	Origin string
}

const (
	OriginProfile    = "profile"
	OriginRobots     = "robots.txt"
	OriginSitemapXml = "sitemap.xml"
	OriginSitemapIdx = "sitemap index"
	OriginHTML       = "HTML"
	OriginWordlist   = "brute-force wordlist"
)
