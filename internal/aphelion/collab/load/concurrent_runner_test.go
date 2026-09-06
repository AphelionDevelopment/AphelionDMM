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

func concurrentService(t *testing.T, scenario loadscenario.ConcurrentScenario) (loadscenario.RunConfig, *server.Service) {
	t.Helper()
	service := server.NewService(server.ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}, PresenceInterval: 16 * time.Millisecond})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	host := httptest.NewServer(service.Handler())
	t.Cleanup(host.Close)
	return createConcurrentSession(t, scenario, service, host.URL), service
}

func createConcurrentSession(t *testing.T, scenario loadscenario.ConcurrentScenario, service *server.Service, endpoint string) loadscenario.RunConfig {
	t.Helper()
	token, err := service.NewLaunchTokenForSnapshot(scenario.Initial)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"snapshot": scenario.Initial})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status %d", response.StatusCode)
	}
	var created server.CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return loadscenario.RunConfig{Endpoint: endpoint, Origin: "http://127.0.0.1", SessionID: created.SessionID, OwnerToken: created.OwnerToken}
}

func TestConcurrentRunnerAppliesEveryClientAndCountsConflicts(t *testing.T) {
	scenario, err := loadscenario.GenerateConcurrent(loadscenario.ConcurrentConfig{Config: loadscenario.Config{Seed: 260906, Clients: 3, Operations: 12, PresencePerClient: 8, MaxX: 4, MaxY: 3, TargetOperationsPerSecond: 100, TargetPresencePerSecondPerEditor: 1000}, ConflictPairs: 3})
	if err != nil {
		t.Fatal(err)
	}
	config, _ := concurrentService(t, scenario)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := loadscenario.RunConcurrent(ctx, config, scenario)
	if err != nil {
		t.Fatalf("run: %v; partial=%#v", err, result)
	}
	if !result.GatePassed || result.ScheduledOperations != 12 || result.SentOperations != 12 || result.AcceptedOperations != 9 || result.RejectedOperations != 3 || result.AppliedDeliveries != 27 {
		t.Fatalf("incorrect operation counts: %#v", result)
	}
	if len(result.Clients) != 3 {
		t.Fatal("missing client convergence evidence")
	}
	for _, peer := range result.Clients {
		if peer.Revision != 9 || peer.MapHash != scenario.ExpectedMapHash || peer.AppliedOperations != 9 {
			t.Fatalf("client did not apply correct stream: %#v", peer)
		}
	}
	if result.SendToAcknowledgement.Count != 9 || result.ScheduleToAcknowledgement.Count != 9 || result.SendToAllApplied.Count != 9 || result.SendToRejection.Count != 3 {
		t.Fatal("latency samples do not correspond to actual outcomes")
	}
	if result.PresenceSent+result.PresenceCoalesced != 24 || result.PresenceCoalesced == 0 {
		t.Fatalf("presence accounting lost negotiated-rate coalescing: %#v", result)
	}
	if result.PresenceObservedDeliveries+result.PresenceUnobservedDeliveries != result.PresenceSent*3 {
		t.Fatal("presence delivery accounting differs")
	}
}
