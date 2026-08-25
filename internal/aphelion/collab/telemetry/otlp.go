package telemetry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Shutdown func(context.Context) error

func NewOTLP(ctx context.Context, endpoint string) (*Telemetry, Shutdown, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, nil, fmt.Errorf("OTLP endpoint must be an HTTPS origin")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: parsed.Hostname()}
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(parsed.Host), otlptracehttp.WithTLSClientConfig(tlsConfig.Clone()))
	if err != nil {
		return nil, nil, fmt.Errorf("initialize OTLP trace exporter: %w", err)
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(parsed.Host), otlpmetrichttp.WithTLSClientConfig(tlsConfig.Clone()))
	if err != nil {
		_ = traceExporter.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("initialize OTLP metric exporter: %w", err)
	}
	serviceResource, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", "apheliondmm-hosted")))
	if err != nil {
		_ = metricExporter.Shutdown(context.Background())
		_ = traceExporter.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("initialize telemetry resource: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(serviceResource))
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(15*time.Second), sdkmetric.WithTimeout(5*time.Second))), sdkmetric.WithResource(serviceResource))
	observability, err := New(Config{TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		_ = meterProvider.Shutdown(context.Background())
		_ = tracerProvider.Shutdown(context.Background())
		return nil, nil, fmt.Errorf("initialize collaboration telemetry: %w", err)
	}
	shutdown := func(ctx context.Context) error {
		return errors.Join(meterProvider.Shutdown(ctx), tracerProvider.Shutdown(ctx))
	}
	return observability, shutdown, nil
}
