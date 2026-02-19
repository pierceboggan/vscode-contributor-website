package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/vscode-contributor-website/heygen"
	"github.com/vscode-contributor-website/scraper"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates *template.Template

func init() {
	templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
}

// Kudos store
var (
	kudosMu    sync.RWMutex
	kudosStore = make(map[string]int)
	validUser  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`)
)

// View models
type ContributorsPageData struct {
	Versions     []VersionOption
	Selected     string
	Contributors []ContributorView
	Loading      bool
}

type VersionOption struct {
	ID       string
	Display  string
	Selected bool
}

type ContributorView struct {
	Name           string
	GitHubUser     string
	AvatarURL      string
	PRs            []PRView
	Kudos          int
	TotalPRCount   int  // Total PRs across all releases
	Milestone      int  // Current milestone reached (5, 10, 25, etc.)
	ShowCelebrate  bool // Whether to show celebrate button
}

type PRView struct {
	Title  string
	URL    string
	Repo   string
	Number string
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "home.html", nil); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "about.html", nil); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

func AskHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "ask.html", nil); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

func ContributorsHandler(w http.ResponseWriter, r *http.Request) {
	availableVersions := scraper.GetAvailableVersions()

	data := ContributorsPageData{}

	if len(availableVersions) == 0 {
		data.Loading = true
		if err := templates.ExecuteTemplate(w, "contributors.html", data); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// Determine selected version
	selectedVersion := r.URL.Query().Get("version")
	if selectedVersion == "" {
		// Default to the latest version that actually has contributors
		for _, v := range availableVersions {
			rel, ok := scraper.GetRelease(v.ID)
			if ok && len(rel.Contributors) > 0 {
				selectedVersion = v.ID
				break
			}
		}
		if selectedVersion == "" {
			selectedVersion = availableVersions[0].ID
		}
	}

	// Build version options from all available versions
	for _, v := range availableVersions {
		data.Versions = append(data.Versions, VersionOption{
			ID:       v.ID,
			Display:  v.Display,
			Selected: v.ID == selectedVersion,
		})
	}

	// Fetch the selected release (on-demand if not cached)
	selectedRelease, ok := scraper.GetRelease(selectedVersion)
	if !ok {
		// Fallback to first available
		selectedVersion = availableVersions[0].ID
		selectedRelease, _ = scraper.GetRelease(selectedVersion)
		if len(data.Versions) > 0 {
			for i := range data.Versions {
				data.Versions[i].Selected = data.Versions[i].ID == selectedVersion
			}
		}
	}
	data.Selected = selectedRelease.DisplayName

	// Calculate total PR counts across all releases for milestone detection
	totalPRCounts := make(map[string]int)
	for _, v := range availableVersions {
		rel, ok := scraper.GetRelease(v.ID)
		if !ok {
			continue
		}
		for _, c := range rel.Contributors {
			totalPRCounts[c.GitHubUser] += len(c.PRs)
		}
	}

	// Build contributor views with kudos counts and milestone info
	kudosMu.RLock()
	for _, c := range selectedRelease.Contributors {
		totalPRs := totalPRCounts[c.GitHubUser]
		milestone := 0
		for _, m := range heygen.Milestones {
			if totalPRs >= m {
				milestone = m
			}
		}

		cv := ContributorView{
			Name:          c.Name,
			GitHubUser:    c.GitHubUser,
			AvatarURL:     c.AvatarURL,
			Kudos:         kudosStore[c.GitHubUser],
			TotalPRCount:  totalPRs,
			Milestone:     milestone,
			ShowCelebrate: milestone >= 5 && heygenClient.IsConfigured(),
		}
		for _, pr := range c.PRs {
			cv.PRs = append(cv.PRs, PRView{
				Title:  pr.Title,
				URL:    pr.URL,
				Repo:   pr.Repo,
				Number: pr.Number,
			})
		}
		data.Contributors = append(data.Contributors, cv)
	}
	kudosMu.RUnlock()

	if err := templates.ExecuteTemplate(w, "contributors.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}

func KudosHandler(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/kudos/")
	if username == "" || !validUser.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "POST":
		kudosMu.Lock()
		kudosStore[username]++
		count := kudosStore[username]
		kudosMu.Unlock()
		fmt.Fprintf(w, `{"count":%d}`, count)
	case "GET":
		kudosMu.RLock()
		count := kudosStore[username]
		kudosMu.RUnlock()
		fmt.Fprintf(w, `{"count":%d}`, count)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Celebrate video store
var (
	celebrateMu    sync.RWMutex
	celebrateStore = make(map[string]string) // username -> videoID
)

var heygenClient = heygen.NewClient()

// CelebrateHandler handles celebration video generation
func CelebrateHandler(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/celebrate/")
	if username == "" || !validUser.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "POST":
		// Generate a new celebration video
		if !heygenClient.IsConfigured() {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "HeyGen API not configured",
				"configured": false,
			})
			return
		}

		// Parse request body for contributor details
		var req struct {
			ContributorName string `json:"contributor_name"`
			Milestone       int    `json:"milestone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Milestone == 0 {
			req.Milestone = 5 // default milestone
		}

		resp, err := heygenClient.GenerateVideo(heygen.GenerateVideoRequest{
			ContributorName: req.ContributorName,
			GitHubUsername:  username,
			Milestone:       req.Milestone,
		})
		if err != nil {
			log.Printf("HeyGen error for %s: %v", username, err)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Failed to generate video",
			})
			return
		}

		// Store video ID for status polling
		celebrateMu.Lock()
		celebrateStore[username] = resp.VideoID
		celebrateMu.Unlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"video_id": resp.VideoID,
			"status":   "pending",
		})

	case "GET":
		// Check video status
		videoID := r.URL.Query().Get("video_id")
		if videoID == "" {
			celebrateMu.RLock()
			videoID = celebrateStore[username]
			celebrateMu.RUnlock()
		}

		if videoID == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "not_found",
			})
			return
		}

		status, err := heygenClient.GetVideoStatus(videoID)
		if err != nil {
			log.Printf("HeyGen status error: %v", err)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "Failed to get video status",
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"video_id":  videoID,
			"status":    status.Status,
			"video_url": status.VideoURL,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// CheckMilestone returns milestone info for a contributor
func CheckMilestone(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/api/milestone/")
	if username == "" || !validUser.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get contributor's PR count across all versions
	allVersions := scraper.GetAvailableVersions()
	prCount := 0
	contributorName := username

	for _, v := range allVersions {
		rel, ok := scraper.GetRelease(v.ID)
		if !ok {
			continue
		}
		for _, c := range rel.Contributors {
			if strings.EqualFold(c.GitHubUser, username) {
				prCount += len(c.PRs)
				if c.Name != "" {
					contributorName = c.Name
				}
			}
		}
	}

	// Find the milestone they've reached
	milestone := 0
	for _, m := range heygen.Milestones {
		if prCount >= m {
			milestone = m
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"username":         username,
		"contributor_name": contributorName,
		"pr_count":         prCount,
		"milestone":        milestone,
		"is_milestone":     heygen.IsMilestone(prCount),
		"configured":       heygenClient.IsConfigured(),
	})
}

// Leaderboard data types
type LeaderboardEntry struct {
	Rank       int
	Name       string
	GitHubUser string
	AvatarURL  string
	PRCount    int
	Releases   int
}

type LeaderboardPageData struct {
	Tab     string // "prs" or "releases"
	Entries []LeaderboardEntry
	Loading bool
}

func LeaderboardHandler(w http.ResponseWriter, r *http.Request) {
	availableVersions := scraper.GetAvailableVersions()

	if len(availableVersions) == 0 {
		data := LeaderboardPageData{Loading: true, Tab: "prs"}
		if err := templates.ExecuteTemplate(w, "leaderboard.html", data); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab != "releases" {
		tab = "prs"
	}

	// Aggregate across all cached releases
	type userStats struct {
		Name       string
		GitHubUser string
		AvatarURL  string
		PRCount    int
		Releases   map[string]bool
	}

	statsMap := make(map[string]*userStats)

	for _, v := range availableVersions {
		rel, ok := scraper.GetRelease(v.ID)
		if !ok || len(rel.Contributors) == 0 {
			continue
		}
		for _, c := range rel.Contributors {
			s, exists := statsMap[c.GitHubUser]
			if !exists {
				s = &userStats{
					Name:       c.Name,
					GitHubUser: c.GitHubUser,
					AvatarURL:  c.AvatarURL,
					Releases:   make(map[string]bool),
				}
				statsMap[c.GitHubUser] = s
			}
			s.PRCount += len(c.PRs)
			s.Releases[v.ID] = true
			// Keep the most recent name/avatar
			if c.Name != "" {
				s.Name = c.Name
			}
			if c.AvatarURL != "" {
				s.AvatarURL = c.AvatarURL
			}
		}
	}

	entries := make([]LeaderboardEntry, 0, len(statsMap))
	for _, s := range statsMap {
		entries = append(entries, LeaderboardEntry{
			Name:       s.Name,
			GitHubUser: s.GitHubUser,
			AvatarURL:  s.AvatarURL,
			PRCount:    s.PRCount,
			Releases:   len(s.Releases),
		})
	}

	if tab == "releases" {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Releases != entries[j].Releases {
				return entries[i].Releases > entries[j].Releases
			}
			return entries[i].PRCount > entries[j].PRCount
		})
	} else {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].PRCount != entries[j].PRCount {
				return entries[i].PRCount > entries[j].PRCount
			}
			return entries[i].Releases > entries[j].Releases
		})
	}

	// Assign ranks and limit to top 50
	limit := 50
	if limit > len(entries) {
		limit = len(entries)
	}
	entries = entries[:limit]
	for i := range entries {
		entries[i].Rank = i + 1
	}

	data := LeaderboardPageData{
		Tab:     tab,
		Entries: entries,
	}

	if err := templates.ExecuteTemplate(w, "leaderboard.html", data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Template error: %v", err)
	}
}