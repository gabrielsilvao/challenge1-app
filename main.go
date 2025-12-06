package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabrielsilvao/challenge1-app/pkg/middleware"
	"github.com/gabrielsilvao/challenge1-app/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	templates *template.Template
	tel       *telemetry.Telemetry
)

func init() {
	templates = template.Must(template.ParseGlob("templates/*.html"))
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize telemetry
	cfg := telemetry.NewConfig()
	var err error
	tel, err = telemetry.Initialize(ctx, cfg)
	if err != nil {
		log.Printf("Warning: Failed to initialize telemetry: %v (continuing without telemetry)", err)
	} else {
		log.Printf("Telemetry initialized - Service: %s, Endpoint: %s", cfg.ServiceName, cfg.OTLPEndpoint)
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := tel.Shutdown(shutdownCtx); err != nil {
				log.Printf("Error shutting down telemetry: %v", err)
			}
		}()
	}

	// Create router
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/echo", echoHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readinessHandler)

	// Apply middleware
	var handler http.Handler = mux
	if tel != nil {
		handler = middleware.TracingMiddleware(tel, mux)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal %v, initiating graceful shutdown...", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
		cancel()
	}()

	log.Printf("Server starting on port %s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped gracefully")
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Create child span for template rendering
	if tel != nil {
		var span trace.Span
		ctx, span = tel.Tracer.Start(ctx, "render-home-template",
			trace.WithAttributes(
				attribute.String("template.name", "index.html"),
			),
		)
		defer span.End()
	}

	if r.URL.Path != "/" {
		if tel != nil {
			span := trace.SpanFromContext(ctx)
			span.SetStatus(codes.Error, "not found")
			span.SetAttributes(attribute.String("error.type", "not_found"))
		}
		http.NotFound(w, r)
		return
	}

	if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		if tel != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, "template execution failed")
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		if tel != nil {
			span := trace.SpanFromContext(ctx)
			span.SetAttributes(
				attribute.String("redirect.reason", "method_not_allowed"),
				attribute.String("http.method", r.Method),
			)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Parse form with tracing
	if tel != nil {
		_, parseSpan := tel.Tracer.Start(ctx, "parse-form")
		if err := r.ParseForm(); err != nil {
			parseSpan.RecordError(err)
			parseSpan.SetStatus(codes.Error, "form parse failed")
			parseSpan.End()
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		parseSpan.End()
	}

	message := r.FormValue("message")
	messageLen := len(message)

	// Record span attributes
	if tel != nil {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(
			attribute.Int("echo.message_length", messageLen),
			attribute.Bool("echo.message_empty", messageLen == 0),
		)

		// Record message length metric
		tel.RecordMessageLength(ctx, messageLen)
	}

	// Create child span for template rendering
	if tel != nil {
		var renderSpan trace.Span
		ctx, renderSpan = tel.Tracer.Start(ctx, "render-echo-template",
			trace.WithAttributes(
				attribute.String("template.name", "echo.html"),
				attribute.Int("data.message_length", messageLen),
			),
		)
		defer renderSpan.End()
	}

	data := struct {
		Message string
	}{
		Message: message,
	}

	if err := templates.ExecuteTemplate(w, "echo.html", data); err != nil {
		if tel != nil {
			span := trace.SpanFromContext(ctx)
			span.RecordError(err)
			span.SetStatus(codes.Error, "template execution failed")
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Health check - minimal processing, no tracing overhead
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy","service":"sample-web-app"}`))
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	// Readiness check - can add dependency checks here
	ctx := r.Context()

	ready := true
	checks := map[string]string{
		"templates": "ok",
	}

	// Check if templates are loaded
	if templates == nil {
		ready = false
		checks["templates"] = "not loaded"
	}

	// Check telemetry (optional)
	if tel != nil {
		checks["telemetry"] = "ok"
	} else {
		checks["telemetry"] = "disabled"
	}

	if tel != nil {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(
			attribute.Bool("readiness.ready", ready),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready","checks":{"templates":"ok"}}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not ready","checks":{"templates":"failed"}}`))
	}
}
