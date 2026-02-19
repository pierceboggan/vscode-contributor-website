package copilotapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// cleanResponse removes CLI stats output and keeps just the response text
func cleanResponse(output string) string {
	// Remove lines starting with common CLI output patterns
	lines := strings.Split(output, "\n")
	var cleaned []string
	for _, line := range lines {
		// Skip stats lines
		if strings.HasPrefix(line, "Total usage") ||
			strings.HasPrefix(line, "API time") ||
			strings.HasPrefix(line, "Total session") ||
			strings.HasPrefix(line, "Total code") ||
			strings.HasPrefix(line, "Breakdown by") ||
			strings.HasPrefix(line, " claude-") ||
			strings.HasPrefix(line, " gpt-") ||
			strings.HasPrefix(line, " gemini-") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// stripAnsi removes ANSI escape codes
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// AskHandler handles POST /api/ask requests with a JSON body { "query": "..." }.
// It uses the Copilot CLI directly via subprocess to answer questions.
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

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	// Simple prompt - let Copilot figure out context from cwd
	prompt := "You are a helpful assistant for a VS Code Contributors website. Answer this question concisely: " + req.Query

	// Call copilot CLI directly
	cmd := exec.CommandContext(ctx, "copilot", "-p", prompt, "--allow-all")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("copilotapi: running query: %s", req.Query)

	err := cmd.Run()
	if err != nil {
		log.Printf("copilotapi: CLI error: %v, stderr: %s", err, stderr.String())
		http.Error(w, "Copilot service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Clean up the response
	answer := stripAnsi(stdout.String())
	answer = cleanResponse(answer)
	if answer == "" {
		answer = "I couldn't generate a response. Please try again."
	}

	log.Printf("copilotapi: response length: %d chars", len(answer))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"answer": answer,
	})
}
