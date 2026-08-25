package telemetry

import (
	"context"
	"errors"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTelemetryEmitsRequiredSignalsWithoutSensitiveAttributes(t *testing.T) {
	t.Parallel()

	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	t.Cleanup(func() { _ = tracerProvider.Shutdown(context.Background()) })
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	value, err := New(Config{TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		t.Fatal(err)
	}

	ctx, finishOperation := value.Operation(context.Background(), OperationSubmit)
	finishOperation(nil)
	ctx, finishStore := value.Store(ctx, StoreAppend)
	finishStore(errors.New("secret-token-and-map-content"))
	ctx, finishReplay := value.Replay(ctx, 3)
	finishReplay(nil)
	value.PresenceDropped(ctx)
	value.ConnectionChanged(ctx, 1)
	value.ConnectionChanged(ctx, -1)

	spanNames := make(map[string]bool)
	for _, span := range spanExporter.GetSpans() {
		spanNames[span.Name] = true
		for _, attribute := range span.Attributes {
			if attribute.Key == "token" || attribute.Key == "map" || attribute.Value.AsString() == "secret-token-and-map-content" {
				t.Fatalf("span %q contains sensitive attribute %q=%q", span.Name, attribute.Key, attribute.Value.AsString())
			}
		}
	}
	for _, name := range []string{"aphelion.collab.operation", "aphelion.collab.store", "aphelion.collab.replay"} {
		if !spanNames[name] {
			t.Errorf("missing span %q", name)
		}
	}

	var collected metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	metricNames := make(map[string]bool)
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			metricNames[metric.Name] = true
		}
	}
	for _, name := range []string{
		"aphelion.collab.operations",
		"aphelion.collab.store.operations",
		"aphelion.collab.replay.operations",
		"aphelion.collab.presence.dropped",
		"aphelion.collab.connections",
	} {
		if !metricNames[name] {
			t.Errorf("missing metric %q", name)
		}
	}
}

func TestDefaultTelemetryHasNoExporterOrNetworkDependency(t *testing.T) {
	t.Parallel()

	value, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, finish := value.Operation(context.Background(), OperationSubmit)
	finish(nil)
	value.PresenceDropped(ctx)
}

func BenchmarkDisabledTelemetry(b *testing.B) {
	ctx := context.Background()
	var value *Telemetry
	b.ReportAllocs()
	for b.Loop() {
		finish := func(error) {}
		if value != nil {
			_, finish = value.Operation(ctx, OperationSubmit)
		}
		finish(nil)
	}
}

func BenchmarkNoExporterTelemetry(b *testing.B) {
	value, err := New(Config{})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		_, finish := value.Operation(ctx, OperationSubmit)
		finish(nil)
	}
}
