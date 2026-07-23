package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/unsubble/searchit/internal/semver"
)

const (
	repoOwner = "UnSubble"
	repoName  = "searchit"
)

var apiBase = "https://api.github.com"

type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
}

type Tag struct {
	Name string `json:"name"`
}

type Client struct {
	HTTPClient *http.Client
	BaseURL    string
}

func NewClient() *Client {
	base := apiBase
	if envBase := os.Getenv("SEARCHIT_API_BASE"); envBase != "" {
		base = envBase
	}
	return &Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		BaseURL:    base,
	}
}

// FetchVersions returns all known versions (stable and experimental) sorted highest to lowest.
// It implements the fallback chain: GitHub Releases -> GitHub Tags -> FAILURE.
func (c *Client) FetchVersions() ([]semver.Version, error) {
	// 1. Try fetching releases
	versions, err := c.fetchReleases()
	if err == nil && len(versions) > 0 {
		return sortVersions(versions), nil
	}

	// 2. Fallback to fetching tags
	versions, err = c.fetchTags()
	if err == nil && len(versions) > 0 {
		return sortVersions(versions), nil
	}

	// 3. FAILURE
	return nil, fmt.Errorf("failed to fetch versions from GitHub (releases and tags failed)")
}

func (c *Client) fetchReleases() ([]semver.Version, error) {
	releasesURL := fmt.Sprintf("%s/repos/%s/%s/releases", c.BaseURL, repoOwner, repoName)
	req, _ := http.NewRequest("GET", releasesURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, err
	}

	var versions []semver.Version
	for _, r := range releases {
		// Ignore drafts
		if r.Draft {
			continue
		}
		if v, err := semver.Parse(r.TagName); err == nil {
			versions = append(versions, v)
		}
	}

	return versions, nil
}

func (c *Client) fetchTags() ([]semver.Version, error) {
	tagsURL := fmt.Sprintf("%s/repos/%s/%s/tags", c.BaseURL, repoOwner, repoName)
	req, _ := http.NewRequest("GET", tagsURL, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tags []Tag
	if err := json.Unmarshal(body, &tags); err != nil {
		return nil, err
	}

	var versions []semver.Version
	for _, t := range tags {
		if v, err := semver.Parse(t.Name); err == nil {
			versions = append(versions, v)
		}
	}

	return versions, nil
}

func sortVersions(versions []semver.Version) []semver.Version {
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(versions[j]) > 0
	})
	return versions
}

// GetLatestStable returns the latest stable version.
func (c *Client) GetLatestStable() (semver.Version, error) {
	versions, err := c.FetchVersions()
	if err != nil {
		return semver.Version{}, err
	}

	for _, v := range versions {
		if v.IsStable() {
			return v, nil
		}
	}

	return semver.Version{}, fmt.Errorf("no stable release found")
}

// GetLatest returns the absolute latest version (could be experimental).
func (c *Client) GetLatest() (semver.Version, error) {
	versions, err := c.FetchVersions()
	if err != nil || len(versions) == 0 {
		return semver.Version{}, fmt.Errorf("no releases found")
	}

	return versions[0], nil
}
