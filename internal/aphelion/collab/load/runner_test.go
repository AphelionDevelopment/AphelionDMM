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

func TestRunnerUsesPublicContractsAndConverges(t *testing.T) {
	scenario, err := loadscenario.Generate(loadscenario.Config{Seed: 42, Clients: 3, Operations: 6, PresencePerClient: 2, MaxX: 3, MaxY: 2})
	if err != nil {
		t.Fatal(err)
	}
	service := server.NewService(server.ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchTokenForSnapshot(scenario.Initial)
	if err != nil {
		t.Fatal(err)
	}
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)
	requestBody, err := json.Marshal(map[string]any{"snapshot": scenario.Initial})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, testServer.URL+"/v1/sessions", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+launchToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var created server.CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := loadscenario.Run(ctx, loadscenario.RunConfig{Endpoint: testServer.URL, Origin: "http://127.0.0.1", SessionID: created.SessionID, OwnerToken: created.OwnerToken}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.AcceptedOperations != len(scenario.Operations) || result.FinalRevision != scenario.ExpectedRevision || result.FinalMapHash != scenario.ExpectedMapHash || result.Diverged {
		t.Fatalf("result = %#v", result)
	}
}
