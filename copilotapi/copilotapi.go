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
	"syscall"
	"time"
)

// cleanResponse removes CLI stats output and tool call traces, keeps just the response text
func cleanResponse(output string) string {
	lines := strings.Split(output, "\n")
	var cleaned []string
	skipNextIfIndented := false
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip stats lines
		if strings.HasPrefix(trimmed, "Total usage") ||
			strings.HasPrefix(trimmed, "API time") ||
			strings.HasPrefix(trimmed, "Total session") ||
			strings.HasPrefix(trimmed, "Total code") ||
			strings.HasPrefix(trimmed, "Breakdown by") ||
			strings.HasPrefix(trimmed, "claude-") ||
			strings.HasPrefix(trimmed, "gpt-") ||
			strings.HasPrefix(trimmed, "gemini-") {
			continue
		}
		
		// Skip tool call output lines (● Read, ● Grep, └ results, etc.)
		if strings.HasPrefix(trimmed, "●") ||
			strings.HasPrefix(trimmed, "└") ||
			strings.HasPrefix(trimmed, "├") ||
			strings.HasPrefix(line, " ●") ||
			strings.HasPrefix(line, " └") ||
			strings.HasPrefix(line, " ├") {
			skipNextIfIndented = true
			continue
		}
		
		// Skip indented continuation lines after tool calls
		if skipNextIfIndented && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			continue
		}
		skipNextIfIndented = false
		
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

	// Use a background context with manual timeout to avoid HTTP request cancellation
	// killing the copilot process prematurely
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Simple prompt - let Copilot figure out context from cwd
	prompt := "You are a helpful assistant for a VS Code Contributors website. Answer this question concisely: " + req.Query

	// Call copilot CLI directly
	cmd := exec.CommandContext(ctx, "copilot", "-p", prompt, "--allow-all")
	// Create new process group so signals don't propagate from parent
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("copilotapi: running query: %s", req.Query)

	err := cmd.Run()
	if err != nil {
		log.Printf("copilotapi: CLI error: %v, stderr: %s, stdout: %s", err, stderr.String(), stdout.String())
		// Check if we got partial output that might be useful
		if stdout.Len() > 0 {
			answer := stripAnsi(stdout.String())
			answer = cleanResponse(answer)
			if len(answer) > 50 {
				// Got substantial output before error, use it
				log.Printf("copilotapi: using partial response (%d chars)", len(answer))
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"answer": answer,
				})
				return
			}
		}
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
