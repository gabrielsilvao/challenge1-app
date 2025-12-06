package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds the telemetry configuration
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
	Insecure       bool
}

// Telemetry holds all telemetry providers and instruments
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter

	// Custom metrics
	RequestCounter   metric.Int64Counter
	RequestDuration  metric.Float64Histogram
	ActiveRequests   metric.Int64UpDownCounter
	ErrorCounter     metric.Int64Counter
	MessageLength    metric.Int64Histogram
}

// NewConfig creates a new telemetry config from environment variables
func NewConfig() *Config {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4318"
	}

	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "sample-web-app"
	}

	serviceVersion := os.Getenv("OTEL_SERVICE_VERSION")
	if serviceVersion == "" {
		serviceVersion = "1.0.0"
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	insecure := os.Getenv("OTEL_INSECURE") != "false"

	return &Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		Environment:    env,
		OTLPEndpoint:   endpoint,
		Insecure:       insecure,
	}
}

// Initialize sets up OpenTelemetry with tracing and metrics
func Initialize(ctx context.Context, cfg *Config) (*Telemetry, error) {
	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
			attribute.String("telemetry.sdk.language", "go"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize trace provider
	tp, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tracer provider: %w", err)
	}

	// Initialize meter provider
	mp, err := initMeterProvider(ctx, cfg, res)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize meter provider: %w", err)
	}

	// Set global providers
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create tracer and meter
	tracer := tp.Tracer(cfg.ServiceName)
	meter := mp.Meter(cfg.ServiceName)

	// Create telemetry instance
	tel := &Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		Tracer:         tracer,
		Meter:          meter,
	}

	// Initialize custom metrics
	if err := tel.initMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	return tel, nil
}

// initTracerProvider creates and configures the trace provider
func initTracerProvider(ctx context.Context, cfg *Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	return tp, nil
}

// initMeterProvider creates and configures the meter provider
func initMeterProvider(ctx context.Context, cfg *Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.OTLPEndpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(15*time.Second),
			),
		),
		sdkmetric.WithResource(res),
	)

	return mp, nil
}

// initMetrics initializes all custom metrics
func (t *Telemetry) initMetrics() error {
	var err error

	// Request counter - counts total HTTP requests
	t.RequestCounter, err = t.Meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	// Request duration histogram
	t.RequestDuration, err = t.Meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return err
	}

	// Active requests gauge
	t.ActiveRequests, err = t.Meter.Int64UpDownCounter(
		"http_requests_active",
		metric.WithDescription("Number of active HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return err
	}

	// Error counter
	t.ErrorCounter, err = t.Meter.Int64Counter(
		"http_errors_total",
		metric.WithDescription("Total number of HTTP errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return err
	}

	// Message length histogram (specific to echo endpoint)
	t.MessageLength, err = t.Meter.Int64Histogram(
		"echo_message_length",
		metric.WithDescription("Length of echo messages"),
		metric.WithUnit("{character}"),
		metric.WithExplicitBucketBoundaries(0, 10, 50, 100, 500, 1000, 5000),
	)
	if err != nil {
		return err
	}

	return nil
}

// Shutdown gracefully shuts down telemetry providers
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error

	if t.TracerProvider != nil {
		if err := t.TracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
		}
	}

	if t.MeterProvider != nil {
		if err := t.MeterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

// RecordRequest records metrics for an HTTP request
func (t *Telemetry) RecordRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.route", path),
		attribute.Int("http.status_code", statusCode),
	}

	t.RequestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	t.RequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if statusCode >= 400 {
		t.ErrorCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// StartRequest increments active requests
func (t *Telemetry) StartRequest(ctx context.Context) {
	t.ActiveRequests.Add(ctx, 1)
}

// EndRequest decrements active requests
func (t *Telemetry) EndRequest(ctx context.Context) {
	t.ActiveRequests.Add(ctx, -1)
}

// RecordMessageLength records the length of echo messages
func (t *Telemetry) RecordMessageLength(ctx context.Context, length int) {
	t.MessageLength.Record(ctx, int64(length))
}
