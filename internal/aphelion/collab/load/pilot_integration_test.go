//go:build pilot

package load_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	loadscenario "sdmm/internal/aphelion/collab/load"
	"sdmm/internal/aphelion/collab/server"
)

func TestPilotScenarioThroughPublicContracts(t *testing.T) {
	config := loadscenario.Config{
		Seed: 20260825, Clients: 25, Operations: 250, PresencePerClient: 20, MaxX: 10, MaxY: 10,
		TargetOperationsPerSecond: 10, TargetPresencePerSecondPerEditor: 20, MaximumP95AcknowledgementMilliseconds: 250,
	}
	scenario, err := loadscenario.Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	service := server.NewService(server.ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchTokenForSnapshot(scenario.Initial)
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(service.Handler())
	t.Cleanup(host.Close)
	encoded, err := json.Marshal(map[string]any{"snapshot": scenario.Initial})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, host.URL+"/v1/sessions", bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+launchToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := host.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var created server.CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d", response.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := loadscenario.Run(ctx, loadscenario.RunConfig{Endpoint: host.URL, Origin: "http://127.0.0.1", SessionID: created.SessionID, OwnerToken: created.OwnerToken}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pilot result: clients=%d operations=%d presence=%d p50=%.2fms p95=%.2fms p99=%.2fms revision=%d hash=%s", result.Clients, result.AcceptedOperations, result.PresenceUpdatesSent, result.P50AcknowledgementMillis, result.P95AcknowledgementMillis, result.P99AcknowledgementMillis, result.FinalRevision, result.FinalMapHash)
	if !result.GatePassed {
		t.Fatalf("pilot gate failed: %#v", result)
	}
}
