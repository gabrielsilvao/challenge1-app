package middleware

import (
	"net/http"
	"time"

	"github.com/gabrielsilvao/challenge1-app/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// TracingMiddleware adds tracing and metrics to HTTP handlers
func TracingMiddleware(tel *telemetry.Telemetry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Track active requests
		tel.StartRequest(r.Context())
		defer tel.EndRequest(r.Context())

		// Create a span for this request
		ctx, span := tel.Tracer.Start(r.Context(), r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.host", r.Host),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.remote_addr", r.RemoteAddr),
				attribute.String("http.scheme", getScheme(r)),
			),
		)
		defer span.End()

		// Wrap response writer to capture status code
		rw := newResponseWriter(w)

		// Add trace ID to response headers
		traceID := span.SpanContext().TraceID().String()
		rw.Header().Set("X-Trace-ID", traceID)

		// Call the next handler
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Record span attributes after handler execution
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int("http.status_code", rw.statusCode),
			attribute.Int64("http.response_size", rw.written),
			attribute.Float64("http.duration_ms", float64(duration.Milliseconds())),
		)

		// Set span status based on HTTP status code
		if rw.statusCode >= 400 {
			span.SetAttributes(attribute.Bool("error", true))
		}

		// Record metrics
		tel.RecordRequest(ctx, r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// getScheme returns the request scheme (http or https)
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}
