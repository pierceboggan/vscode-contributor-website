package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PR struct {
	Title  string
	URL    string
	Repo   string
	Number string
}

type Contributor struct {
	Name       string
	GitHubUser string
	AvatarURL  string
	PRs        []PR
}

// ContributorSearchResult represents aggregated contributor data across releases.
type ContributorSearchResult struct {
	GitHubUser   string
	Name         string
	AvatarURL    string
	TotalPRs     int
	ReleaseCount int
}

type Release struct {
	Version      string
	DisplayName  string
	Contributors []Contributor
}

// ContributorHistory aggregates a contributor's activity across all releases.
type ContributorHistory struct {
	GitHubUser    string
	Name          string
	AvatarURL     string
	TotalPRs      int
	ReleaseCount  int
	FirstRelease  string            // version of first contribution
	LatestRelease string            // version of most recent contribution
	PRsByRelease  map[string][]PR   // version -> PRs
}

// VersionInfo holds a version identifier and its display name.
type VersionInfo struct {
	ID      string // e.g. "v1_109"
	Display string // e.g. "1.109"
}

var (
	mu     sync.RWMutex
	cached = make(map[string]Release)

	versionsMu        sync.RWMutex
	availableVersions []VersionInfo
)

// fallbackVersions is used when the GitHub API is unavailable.
var fallbackVersions = []string{
	"v1_109", "v1_108", "v1_107", "v1_106", "v1_105",
}

// prefetchCount is the number of recent versions to pre-fetch on startup.
const prefetchCount = 5

var client = &http.Client{Timeout: 30 * time.Second}

// GetAvailableVersions returns all known release versions (newest first).
func GetAvailableVersions() []VersionInfo {
	versionsMu.RLock()
	defer versionsMu.RUnlock()
	return availableVersions
}

// GetRelease returns a single release, fetching on-demand if not cached.
func GetRelease(version string) (Release, bool) {
	mu.RLock()
	r, ok := cached[version]
	mu.RUnlock()
	if ok {
		return r, true
	}

	// Fetch on demand
	rel, err := fetchRelease(version)
	if err != nil {
		log.Printf("scraper: failed to fetch %s: %v", version, err)
		return Release{}, false
	}

	mu.Lock()
	cached[version] = rel
	mu.Unlock()
	return rel, true
}

// GetReleases returns cached releases for the prefetched versions.
func GetReleases() []Release {
	versionsMu.RLock()
	versions := availableVersions
	versionsMu.RUnlock()

	mu.RLock()
	defer mu.RUnlock()

	var results []Release
	for _, v := range versions {
		if r, ok := cached[v.ID]; ok && len(r.Contributors) > 0 {
			results = append(results, r)
		}
	}
	return results
}

// SearchContributors searches all cached releases for contributors matching the query.
// The search is case-insensitive and matches partial GitHub usernames.
func SearchContributors(query string) []ContributorSearchResult {
	mu.RLock()
	defer mu.RUnlock()

	query = strings.ToLower(query)
	if query == "" {
		return nil
	}

	// Aggregate data by GitHub username (lowercase for deduplication)
	type aggregated struct {
		GitHubUser   string
		Name         string
		AvatarURL    string
		TotalPRs     int
		ReleaseCount int
	}
	byUser := make(map[string]*aggregated)

	for _, release := range cached {
		for _, contrib := range release.Contributors {
			userLower := strings.ToLower(contrib.GitHubUser)
			if !strings.Contains(userLower, query) {
				continue
			}

			if agg, exists := byUser[userLower]; exists {
				agg.TotalPRs += len(contrib.PRs)
				agg.ReleaseCount++
			} else {
				byUser[userLower] = &aggregated{
					GitHubUser:   contrib.GitHubUser,
					Name:         contrib.Name,
					AvatarURL:    contrib.AvatarURL,
					TotalPRs:     len(contrib.PRs),
					ReleaseCount: 1,
				}
			}
		}
	}

	// Convert to result slice
	results := make([]ContributorSearchResult, 0, len(byUser))
	for _, agg := range byUser {
		results = append(results, ContributorSearchResult{
			GitHubUser:   agg.GitHubUser,
			Name:         agg.Name,
			AvatarURL:    agg.AvatarURL,
			TotalPRs:     agg.TotalPRs,
			ReleaseCount: agg.ReleaseCount,
		})
	}

	// Sort by TotalPRs descending for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalPRs > results[j].TotalPRs
	})

	return results
}

// GetContributorHistory returns aggregated contribution history for a user.
// Returns nil if the user is not found in any cached release.
func GetContributorHistory(username string) *ContributorHistory {
	mu.RLock()
	defer mu.RUnlock()

	history := &ContributorHistory{
		PRsByRelease: make(map[string][]PR),
	}

	usernameLower := strings.ToLower(username)
	var firstVersion, latestVersion string
	var firstVersionNum, latestVersionNum int

	for version, release := range cached {
		for _, contrib := range release.Contributors {
			if strings.ToLower(contrib.GitHubUser) == usernameLower {
				// Set user info from first match
				if history.GitHubUser == "" {
					history.GitHubUser = contrib.GitHubUser
					history.Name = contrib.Name
					history.AvatarURL = contrib.AvatarURL
				}

				// Add PRs for this release
				if len(contrib.PRs) > 0 {
					history.PRsByRelease[version] = append(history.PRsByRelease[version], contrib.PRs...)
					history.TotalPRs += len(contrib.PRs)
				}
				history.ReleaseCount++

				// Track first and latest release
				vNum := versionNumber(version)
				if firstVersion == "" || vNum < firstVersionNum {
					firstVersion = version
					firstVersionNum = vNum
				}
				if latestVersion == "" || vNum > latestVersionNum {
					latestVersion = version
					latestVersionNum = vNum
				}
				break // Found contributor in this release, move to next
			}
		}
	}

	// Return nil if user not found
	if history.GitHubUser == "" {
		return nil
	}

	history.FirstRelease = firstVersion
	history.LatestRelease = latestVersion
	return history
}

// Refresh discovers available versions and pre-fetches recent ones.
func Refresh() []Release {
	// Discover all available versions
	versions, err := discoverVersions()
	if err != nil {
		log.Printf("scraper: failed to discover versions: %v", err)
		// Use fallback if discovery fails and we have nothing cached
		versionsMu.RLock()
		hasVersions := len(availableVersions) > 0
		versionsMu.RUnlock()
		if !hasVersions {
			versions = toVersionInfos(fallbackVersions)
		} else {
			versionsMu.RLock()
			versions = availableVersions
			versionsMu.RUnlock()
		}
	}

	versionsMu.Lock()
	availableVersions = versions
	versionsMu.Unlock()

	// Pre-fetch the most recent versions
	limit := prefetchCount
	if limit > len(versions) {
		limit = len(versions)
	}
	for _, v := range versions[:limit] {
		r, err := fetchRelease(v.ID)
		if err != nil {
			log.Printf("scraper: failed to fetch %s: %v", v.ID, err)
			continue
		}
		mu.Lock()
		cached[v.ID] = r
		mu.Unlock()
	}

	log.Printf("scraper: discovered %d versions, pre-fetched %d", len(versions), limit)
	return GetReleases()
}

// StartBackground begins periodic scraping in the background.
func StartBackground() {
	go func() {
		Refresh()
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			Refresh()
		}
	}()
}

// discoverVersions lists release note files from the vscode-docs GitHub repo.
func discoverVersions() ([]VersionInfo, error) {
	url := "https://api.github.com/repos/microsoft/vscode-docs/contents/release-notes"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}

	var entries []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	versionFileRe := regexp.MustCompile(`^(v\d+_\d+)\.md$`)
	var versions []VersionInfo
	for _, e := range entries {
		m := versionFileRe.FindStringSubmatch(e.Name)
		if m == nil {
			continue
		}
		id := m[1]
		display := strings.TrimPrefix(id, "v")
		display = strings.Replace(display, "_", ".", 1)
		versions = append(versions, VersionInfo{ID: id, Display: display})
	}

	// Sort by version number descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versionNumber(versions[i].ID) > versionNumber(versions[j].ID)
	})

	return versions, nil
}

func versionNumber(id string) int {
	// "v1_109" -> 1109, "v1_99" -> 199
	s := strings.TrimPrefix(id, "v")
	parts := strings.SplitN(s, "_", 2)
	if len(parts) != 2 {
		return 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	return major*10000 + minor
}

func toVersionInfos(ids []string) []VersionInfo {
	var out []VersionInfo
	for _, id := range ids {
		display := strings.TrimPrefix(id, "v")
		display = strings.Replace(display, "_", ".", 1)
		out = append(out, VersionInfo{ID: id, Display: display})
	}
	return out
}

func fetchRelease(version string) (Release, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/microsoft/vscode-docs/main/release-notes/%s.md", version)

	resp, err := client.Get(url)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return Release{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Release{}, err
	}

	return parseMarkdown(version, string(body)), nil
}

// Regex patterns for markdown parsing.
var (
	// Matches: * [@username (Display Name)](https://github.com/username)...
	contribLineRe = regexp.MustCompile(`^\* \[@([^\]]+)\]\(https://github\.com/([^)]+)\)(.*)$`)
	// Matches indented sub-items
	subItemRe = regexp.MustCompile(`^\s+\* (.+)$`)
	// Matches PR links: [PR #123](https://github.com/org/repo/pull/123)
	prLinkRe = regexp.MustCompile(`\[PR #(\d+)\]\((https://github\.com/([^)]+)/pull/\d+)\)`)
	// Matches repo section headers: Contributions to `repo`:
	repoSectionRe = regexp.MustCompile("^Contributions to `([^`]+)`:?$")
)

func parseMarkdown(version, md string) Release {
	display := strings.TrimPrefix(version, "v")
	display = strings.Replace(display, "_", ".", 1)

	release := Release{
		Version:     version,
		DisplayName: display,
	}

	lines := strings.Split(md, "\n")
	inPRSection := false
	var currentContrib *Contributor

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if strings.HasPrefix(line, "### Pull Requests") {
			inPRSection = true
			continue
		}
		if !inPRSection {
			continue
		}

		// Stop at next major heading
		if strings.HasPrefix(line, "## ") {
			break
		}

		// Repo section header (informational, we get repo from PR URL)
		if repoSectionRe.MatchString(line) {
			currentContrib = nil
			continue
		}

		// Contributor line
		if m := contribLineRe.FindStringSubmatch(line); m != nil {
			displayText := m[1]
			githubUser := m[2]
			rest := m[3]

			name := githubUser
			if idx := strings.Index(displayText, " ("); idx != -1 {
				name = strings.TrimSuffix(displayText[idx+2:], ")")
			}

			c := Contributor{
				Name:       name,
				GitHubUser: githubUser,
				AvatarURL:  fmt.Sprintf("https://github.com/%s.png?size=80", githubUser),
			}

			// Extract PRs from rest of line
			if prMatches := prLinkRe.FindAllStringSubmatch(rest, -1); len(prMatches) > 0 {
				for _, pm := range prMatches {
					desc := extractDescription(rest, pm[0])
					c.PRs = append(c.PRs, PR{
						Title:  desc,
						URL:    pm[2],
						Repo:   pm[3],
						Number: pm[1],
					})
				}
			}

			release.Contributors = append(release.Contributors, c)
			currentContrib = &release.Contributors[len(release.Contributors)-1]
			continue
		}

		// Sub-item (belongs to current contributor)
		if currentContrib != nil {
			if m := subItemRe.FindStringSubmatch(line); m != nil {
				content := m[1]
				if prMatches := prLinkRe.FindAllStringSubmatch(content, -1); len(prMatches) > 0 {
					for _, pm := range prMatches {
						desc := extractDescription(content, pm[0])
						currentContrib.PRs = append(currentContrib.PRs, PR{
							Title:  desc,
							URL:    pm[2],
							Repo:   pm[3],
							Number: pm[1],
						})
					}
				}
			}
		}
	}

	return release
}

func extractDescription(text, prLink string) string {
	idx := strings.Index(text, prLink)
	if idx <= 0 {
		return ""
	}
	desc := strings.TrimSpace(text[:idx])
	desc = strings.TrimPrefix(desc, ": ")
	desc = strings.TrimPrefix(desc, ":")
	desc = strings.TrimSpace(desc)
	return desc
}

// IsFirstTimeContributor returns true if this is the first release where the user contributed.
// It checks all cached releases with versions BEFORE the given version.
func IsFirstTimeContributor(username string, version string) bool {
	versionsMu.RLock()
	versions := availableVersions
	versionsMu.RUnlock()

	targetNum := versionNumber(version)

	mu.RLock()
	defer mu.RUnlock()

	for _, v := range versions {
		// Only check releases BEFORE the given version
		if versionNumber(v.ID) >= targetNum {
			continue
		}
		rel, ok := cached[v.ID]
		if !ok {
			continue
		}
		for _, c := range rel.Contributors {
			if strings.EqualFold(c.GitHubUser, username) {
				// User contributed in an earlier release
				return false
			}
		}
	}
	return true
}
