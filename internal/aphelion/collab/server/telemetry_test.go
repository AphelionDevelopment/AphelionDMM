package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
	collabstore "sdmm/internal/aphelion/collab/store"
	collabtelemetry "sdmm/internal/aphelion/collab/telemetry"
)

func TestServiceEmitsOperationAndConnectionTelemetry(t *testing.T) {
	t.Parallel()

	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	t.Cleanup(func() { _ = tracerProvider.Shutdown(context.Background()) })
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observability, err := collabtelemetry.New(collabtelemetry.Config{TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		t.Fatal(err)
	}
	service, created, testServer := startHTTPTestSessionWithConfig(t, ServiceConfig{
		AllowedOrigins: []string{"http://127.0.0.1"},
		Telemetry:      observability,
	})
	snapshot, err := service.sessions[created.SessionID].owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	connection := connectTestClient(t, testServer.URL, created.SessionID, created.OwnerToken, 0)
	submitTestOperation(t, connection, created.SessionID, "telemetry-operation", testOperation(t, snapshot, 1))
	_ = readServerEnvelopeType(t, connection, protocol.ServerOperationAccepted)
	if err := connection.CloseNow(); err != nil {
		t.Fatal(err)
	}

	spanNames := make(map[string]bool)
	for _, span := range spanExporter.GetSpans() {
		spanNames[span.Name] = true
	}
	for _, name := range []string{"aphelion.collab.operation", "aphelion.collab.store"} {
		if !spanNames[name] {
			t.Errorf("accepted operation emitted no %q span", name)
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
	for _, name := range []string{"aphelion.collab.operations", "aphelion.collab.connections"} {
		if !metricNames[name] {
			t.Errorf("missing integrated metric %q", name)
		}
	}
}

func TestRecoveryEmitsStoreAndReplayTelemetry(t *testing.T) {
	t.Parallel()

	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	t.Cleanup(func() { _ = tracerProvider.Shutdown(context.Background()) })
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observability, err := collabtelemetry.New(collabtelemetry.Config{TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	store := collabstore.NewMemoryStore()
	if err := store.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Store: store, Telemetry: observability})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchTokenForSnapshot(fixture.Initial)
	if err != nil {
		t.Fatal(err)
	}
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)
	created := createTestSession(t, testServer.URL, launchToken, fixture.Initial)
	if created.Revision != fixture.First.Revision {
		t.Fatalf("recovered revision = %d, want %d", created.Revision, fixture.First.Revision)
	}

	spanNames := make(map[string]bool)
	for _, span := range spanExporter.GetSpans() {
		spanNames[span.Name] = true
	}
	for _, name := range []string{"aphelion.collab.store", "aphelion.collab.replay"} {
		if !spanNames[name] {
			t.Errorf("recovery emitted no %q span", name)
		}
	}
}

func TestSnapshotAndPresenceDropEmitTelemetry(t *testing.T) {
	t.Parallel()

	spanExporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	t.Cleanup(func() { _ = tracerProvider.Shutdown(context.Background()) })
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	observability, err := collabtelemetry.New(collabtelemetry.Config{TracerProvider: tracerProvider, MeterProvider: meterProvider})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot(t, 1)
	value := &blockingSnapshotStore{
		MemoryStore: NewMemoryStore(),
		started:     make(chan model.Snapshot, 1),
		release:     make(chan struct{}),
	}
	owner, err := StartDocumentWithConfig(context.Background(), snapshot, value, DocumentConfig{
		SnapshotOperationThreshold: 1,
		Telemetry:                  observability,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	if _, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-value.started:
	case <-time.After(time.Second):
		t.Fatal("snapshot did not start")
	}
	close(value.release)

	manager := NewPresenceManagerWithTelemetry(time.Minute, observability)
	principal := testPrincipal(t, "Editor", RoleEditor)
	_, _, cancel := manager.Subscribe(1)
	defer cancel()
	if err := manager.Update(principal, PresenceUpdate{Sequence: 1, Status: "active"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Update(principal, PresenceUpdate{Sequence: 2, Status: "active"}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		for _, span := range spanExporter.GetSpans() {
			if span.Name == "aphelion.collab.store" {
				goto snapshotObserved
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("snapshot emitted no store span")
		}
		time.Sleep(time.Millisecond)
	}

snapshotObserved:
	var collected metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "aphelion.collab.presence.dropped" {
				sum, ok := metric.Data.(metricdata.Sum[int64])
				if !ok || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
					t.Fatalf("presence drop metric = %#v, want one drop", metric.Data)
				}
				return
			}
		}
	}
	t.Fatal("missing presence drop metric")
}
