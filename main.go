package main

import (
	"embed"
	"fmt"
	"net/http"
)

//go:embed index.html
var indexPage embed.FS

func setupRoutes() {
	http.HandleFunc("GET /", serveHTML)
	http.HandleFunc("POST /", processHandler)
}

func main() {
	setupRoutes()
	fmt.Println("Server in ascolto su :3000")
	http.ListenAndServe(":3000", nil)
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	html, err := indexPage.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Errore durante il caricamento del form HTML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(html)
}
