package main

import (
	"log"
	"net/http"

	"github.com/vscode-contributor-website/copilotapi"
	"github.com/vscode-contributor-website/scraper"
	"github.com/vscode-contributor-website/web"
)

func main() {
	// Start background contributor scraping
	scraper.StartBackground()

	fs := http.FileServer(http.Dir("public/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		web.HomeHandler(w, r)
	})
	http.HandleFunc("/about", web.AboutHandler)
	http.HandleFunc("/contributors", web.ContributorsHandler)
	http.HandleFunc("/leaderboard", web.LeaderboardHandler)
	http.HandleFunc("/api/kudos/", web.KudosHandler)
	http.HandleFunc("/api/ask", copilotapi.AskHandler)

	log.Println("Server starting on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
