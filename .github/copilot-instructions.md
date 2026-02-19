# VS Code Contributors Website

A Go web app that displays community contributors to VS Code releases by scraping release notes from the microsoft/vscode-docs repository.

## Build & Run

```bash
# Run locally (serves on http://localhost:8080)
go run main.go

# Build binary
go build -o vscode-contributor-website
```

No test suite exists currently.

## Architecture

```
main.go           → HTTP server setup, routes to handlers
├── web/          → Web handlers and HTML templates
│   ├── web.go    → All page handlers (Home, About, Contributors, Leaderboard, Kudos API)
│   └── templates/*.html → Embedded HTML templates (go:embed)
├── scraper/      → Background scraper for VS Code release notes
│   └── scraper.go → Fetches/parses contributor data from vscode-docs repo
├── copilotapi/   → Copilot SDK integration for AI-powered queries
│   └── copilotapi.go → Tools exposing contributor data to Copilot agent
└── api/          → Vercel serverless function entrypoints
```

**Key data flow:**
1. `scraper.StartBackground()` launches a goroutine that discovers VS Code versions via GitHub API
2. Recent versions are pre-fetched and parsed from markdown release notes
3. Contributors and PRs are extracted using regex patterns and cached in memory
4. Web handlers render templates with contributor data
5. `/api/ask` provides a Copilot-powered Q&A interface using custom tools

**Templates:** HTML templates are embedded via `//go:embed templates/*.html` in `web/web.go`. Add new templates to `web/templates/` and they'll be auto-included.

## Routes

| Route | Handler | Description |
|-------|---------|-------------|
| `/` | `web.HomeHandler` | Home page |
| `/about` | `web.AboutHandler` | About page |
| `/contributors` | `web.ContributorsHandler` | Contributors list with version selector |
| `/leaderboard` | `web.LeaderboardHandler` | Top contributors by PRs/releases |
| `/api/kudos/{user}` | `web.KudosHandler` | GET/POST kudos for a user |
| `/api/ask` | `copilotapi.AskHandler` | Copilot-powered Q&A |

## Conventions

- **Handler pattern:** Each handler retrieves data from `scraper` package, builds a view model struct, then calls `templates.ExecuteTemplate()`
- **Concurrency:** The scraper uses `sync.RWMutex` for thread-safe cache access; kudos store uses separate mutex
- **API responses:** JSON endpoints use manual `fmt.Fprintf` or `json.NewEncoder` rather than a framework
- **Static files:** Served from `public/static/` at `/static/` path

## Deployment

Configured for Vercel via `vercel.json`. Serverless functions in `api/` directory handle routing rewrites.

## Validation

Use the `webapp-validation` skill to verify features work end-to-end with Playwright. It starts the server, runs browser tests, and captures screenshots.
