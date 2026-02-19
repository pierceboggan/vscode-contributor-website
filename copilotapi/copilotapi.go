package copilotapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/vscode-contributor-website/scraper"
)

// Tool parameter types

type GetContributorsParams struct {
	Version string `json:"version" jsonschema:"VS Code release version ID, e.g. v1_109"`
}

type GetReleasesParams struct {
	Limit int `json:"limit" jsonschema:"Max number of releases to return (default 10)"`
}

type SearchContributorParams struct {
	Username string `json:"username" jsonschema:"GitHub username to search for across releases"`
}

// createTools builds the custom tools that expose our scraper data to the agent.
func createTools() []copilot.Tool {
	getContributors := copilot.DefineTool(
		"get_vscode_contributors",
		"Get the list of community contributors for a specific VS Code release version. Returns contributor names, GitHub usernames, avatar URLs, and their PRs.",
		func(params GetContributorsParams, inv copilot.ToolInvocation) (any, error) {
			release, ok := scraper.GetRelease(params.Version)
			if !ok {
				return nil, fmt.Errorf("release %s not found", params.Version)
			}
			return release.Contributors, nil
		},
	)

	listReleases := copilot.DefineTool(
		"list_vscode_releases",
		"List available VS Code release versions (newest first). Each version has an ID (e.g. v1_109) and display name (e.g. 1.109).",
		func(params GetReleasesParams, inv copilot.ToolInvocation) (any, error) {
			versions := scraper.GetAvailableVersions()
			limit := params.Limit
			if limit <= 0 {
				limit = 10
			}
			if limit > len(versions) {
				limit = len(versions)
			}
			return versions[:limit], nil
		},
	)

	searchContributor := copilot.DefineTool(
		"search_contributor",
		"Search for a specific GitHub user across all cached VS Code releases. Returns the releases they contributed to and their PRs in each.",
		func(params SearchContributorParams, inv copilot.ToolInvocation) (any, error) {
			username := strings.ToLower(params.Username)
			releases := scraper.GetReleases()

			type match struct {
				Version string       `json:"version"`
				PRs     []scraper.PR `json:"prs"`
			}
			var results []match

			for _, rel := range releases {
				for _, c := range rel.Contributors {
					if strings.EqualFold(c.GitHubUser, username) {
						results = append(results, match{
							Version: rel.DisplayName,
							PRs:     c.PRs,
						})
						break
					}
				}
			}

			if len(results) == 0 {
				return fmt.Sprintf("No contributions found for @%s in cached releases", username), nil
			}
			return results, nil
		},
	)

	return []copilot.Tool{getContributors, listReleases, searchContributor}
}

const systemPrompt = `You are a helpful assistant for the VS Code Contributors website.
You help users explore community contribution data for Visual Studio Code releases.

Use the provided tools to look up release versions, contributors, and their pull requests.
When listing contributors, format them clearly with their GitHub username and PR details.
Keep answers concise and well-formatted. Use markdown for structure.

You only have access to data from VS Code release notes. If asked about something outside
this scope, let the user know politely.`

// AskHandler handles POST /api/ask requests with a JSON body { "query": "..." }.
// It creates a Copilot SDK session, sends the query with custom tools, and returns the response.
func AskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	client := copilot.NewClient(&copilot.ClientOptions{
		LogLevel: "error",
	})
	if err := client.Start(ctx); err != nil {
		log.Printf("copilotapi: failed to start client: %v", err)
		http.Error(w, "Copilot service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer client.Stop()

	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		Model: "gpt-4.1",
		Tools: createTools(),
		SystemMessage: &copilot.SystemMessageConfig{
			Content: systemPrompt,
		},
	})
	if err != nil {
		log.Printf("copilotapi: failed to create session: %v", err)
		http.Error(w, "Failed to create Copilot session", http.StatusServiceUnavailable)
		return
	}
	defer session.Destroy()

	// Collect the response
	var answer string
	done := make(chan struct{})

	session.On(func(event copilot.SessionEvent) {
		if event.Type == "assistant.message" && event.Data.Content != nil {
			answer = *event.Data.Content
		}
		if event.Type == "session.idle" {
			close(done)
		}
	})

	if _, err := session.Send(ctx, copilot.MessageOptions{
		Prompt: req.Query,
	}); err != nil {
		log.Printf("copilotapi: failed to send message: %v", err)
		http.Error(w, "Failed to send query", http.StatusInternalServerError)
		return
	}

	select {
	case <-done:
	case <-ctx.Done():
		http.Error(w, "Request timed out", http.StatusGatewayTimeout)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"answer": answer,
	})
}