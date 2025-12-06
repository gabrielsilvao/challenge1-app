package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
)

var templates *template.Template

func init() {
	templates = template.Must(template.ParseGlob("templates/*.html"))
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/echo", echoHandler)
	http.HandleFunc("/health", healthHandler)

	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	templates.ExecuteTemplate(w, "index.html", nil)
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	message := r.FormValue("message")
	data := struct {
		Message string
	}{
		Message: message,
	}

	templates.ExecuteTemplate(w, "echo.html", data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}
