package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestEmbeddedUsesLoopbackEphemeralPortAndSingleUseToken(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	embedded, err := StartEmbedded(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = embedded.Shutdown(context.Background()) })
	launchToken := embedded.TakeLaunchToken()
	if launchToken == "" {
		t.Fatal("TakeLaunchToken() is empty")
	}
	if secondTake := embedded.TakeLaunchToken(); secondTake != "" {
		t.Fatalf("second TakeLaunchToken() = %q, want empty", secondTake)
	}
	host, port, err := net.SplitHostPort(embedded.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Fatalf("embedded host = %q, want loopback", host)
	}
	if port == "0" || port == "" {
		t.Fatalf("embedded port = %q, want resolved ephemeral port", port)
	}

	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	first := postJSON(t, embedded.BaseURL+"/v1/sessions", launchToken, body)
	_ = first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first redemption status = %d, want 201", first.StatusCode)
	}
	second := postJSON(t, embedded.BaseURL+"/v1/sessions", launchToken, body)
	_ = second.Body.Close()
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second redemption status = %d, want 401", second.StatusCode)
	}
}

func TestEmbeddedShutdownIsBoundedAndIdempotent(t *testing.T) {
	t.Parallel()

	embedded, err := StartEmbedded(context.Background(), testSnapshot(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := embedded.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := embedded.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	response, err := http.Get(embedded.BaseURL + "/v1/health/live")
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("health request succeeded after shutdown")
	}
}

func TestEmbeddedLaunchTokenIsScopedToSuppliedSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot(t, 1)
	embedded, err := StartEmbedded(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = embedded.Shutdown(context.Background()) })
	launchToken := embedded.TakeLaunchToken()
	mismatch := snapshot
	mismatch.MaxX = 2
	mismatchBody, err := json.Marshal(map[string]any{"snapshot": mismatch})
	if err != nil {
		t.Fatal(err)
	}
	rejected := postJSON(t, embedded.BaseURL+"/v1/sessions", launchToken, mismatchBody)
	_ = rejected.Body.Close()
	if rejected.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched snapshot status = %d, want 403", rejected.StatusCode)
	}
	correctBody, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	accepted := postJSON(t, embedded.BaseURL+"/v1/sessions", launchToken, correctBody)
	_ = accepted.Body.Close()
	if accepted.StatusCode != http.StatusCreated {
		t.Fatalf("correct snapshot status after mismatch = %d, want 201", accepted.StatusCode)
	}
}

func TestEmbeddedUsesConfiguredStore(t *testing.T) {
	t.Parallel()

	value := NewMemoryStore()
	embedded, err := StartEmbeddedWithConfig(context.Background(), testSnapshot(t, 1), EmbeddedConfig{
		Store:    value,
		Document: DocumentConfig{SnapshotOperationThreshold: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = embedded.Shutdown(context.Background()) })
	if embedded.service.store != value || embedded.service.documentConfig.SnapshotOperationThreshold != 5 {
		t.Fatal("embedded service did not retain durable-store configuration")
	}
}
