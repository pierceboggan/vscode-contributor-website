package handler

import (
	"net/http"

	"github.com/vscode-contributor-website/web"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	web.AboutHandler(w, r)
}
