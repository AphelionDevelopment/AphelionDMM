package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/server"
	collabui "sdmm/internal/aphelion/collab/ui"
)

// CollaborationReport records a real loopback two-client smoke run.
type CollaborationReport struct {
	ServiceLaunch     StepResult       `json:"service_launch"`
	ClientAJoin       StepResult       `json:"client_a_join"`
	ClientBJoin       StepResult       `json:"client_b_join"`
	Operations        StepResult       `json:"operations"`
	Convergence       StepResult       `json:"convergence"`
	Leave             StepResult       `json:"leave"`
	Shutdown          StepResult       `json:"shutdown"`
	SessionIDPresent  bool             `json:"session_id_present"`
	DistinctActors    bool             `json:"distinct_actors"`
	AcceptedRevisions []model.Revision `json:"accepted_revisions,omitempty"`
	ClientAHash       string           `json:"client_a_hash,omitempty"`
	ClientBHash       string           `json:"client_b_hash,omitempty"`
}

// OK reports whether every collaboration smoke step completed successfully.
func (report CollaborationReport) OK() bool {
	return report.ServiceLaunch.OK && report.ClientAJoin.OK && report.ClientBJoin.OK && report.Operations.OK && report.Convergence.OK && report.Leave.OK && report.Shutdown.OK && report.SessionIDPresent && report.DistinctActors && len(report.AcceptedRevisions) == 2 && report.ClientAHash != "" && report.ClientAHash == report.ClientBHash
}

// RunCollaboration starts the real loopback service and verifies two-client convergence.
func RunCollaboration(ctx context.Context, snapshot model.Snapshot) (report CollaborationReport) {
	embedded, err := server.StartEmbedded(ctx, snapshot)
	if err != nil {
		report.ServiceLaunch.Detail = err.Error()
		return report
	}
	report.ServiceLaunch.OK = true
	clientA := collabui.NewSessionClient(collabui.SessionClientConfig{HTTPTimeout: time.Second})
	clientB := collabui.NewSessionClient(collabui.SessionClientConfig{HTTPTimeout: time.Second})
	defer func() {
		leaveErr := errors.Join(clientB.Leave(context.Background()), clientA.Leave(context.Background()))
		if leaveErr != nil {
			report.Leave.Detail = leaveErr.Error()
		} else {
			report.Leave.OK = true
		}
		if shutdownErr := embedded.Shutdown(context.Background()); shutdownErr != nil {
			report.Shutdown.Detail = shutdownErr.Error()
		} else {
			report.Shutdown.OK = true
		}
	}()

	invitationA, err := clientA.Create(ctx, embedded.Endpoint(), embedded.TakeLaunchToken(), snapshot)
	if err != nil {
		report.ClientAJoin.Detail = err.Error()
		return report
	}
	ownerToken := invitationA.Token
	if err := clientA.Join(ctx, invitationA); err != nil {
		report.ClientAJoin.Detail = err.Error()
		return report
	}
	report.ClientAJoin.OK = true
	report.SessionIDPresent = invitationA.SessionID != ""

	tokenB, err := createSmokeJoinToken(ctx, invitationA.BaseURL, invitationA.SessionID, ownerToken)
	invitationA.Token = ""
	if err != nil {
		report.ClientBJoin.Detail = err.Error()
		return report
	}
	invitationB := collabui.Invitation{BaseURL: invitationA.BaseURL, Origin: invitationA.Origin, SessionID: invitationA.SessionID, Token: tokenB}
	if err := clientB.Join(ctx, invitationB); err != nil {
		report.ClientBJoin.Detail = err.Error()
		return report
	}
	invitationB.Token = ""
	report.ClientBJoin.OK = true

	networkA := clientA.NetworkExecutor()
	networkB := clientB.NetworkExecutor()
	if networkA == nil || networkB == nil {
		report.Operations.Detail = "joined client has no network executor"
		return report
	}
	baseA, err := networkA.Snapshot(ctx)
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	operationA, err := smokeOperation(baseA, model.Coord{X: 1, Y: 1, Z: 1})
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	acceptedA, err := networkA.Execute(ctx, operationA)
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	baseB, err := waitForRevision(ctx, networkB.Snapshot, acceptedA.Revision)
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	operationB, err := smokeOperation(baseB, model.Coord{X: 2, Y: 1, Z: 1})
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	acceptedB, err := networkB.Execute(ctx, operationB)
	if err != nil {
		report.Operations.Detail = err.Error()
		return report
	}
	report.AcceptedRevisions = []model.Revision{acceptedA.Revision, acceptedB.Revision}
	report.DistinctActors = acceptedA.ActorID != acceptedB.ActorID
	report.Operations.OK = true

	finalA, err := waitForRevision(ctx, networkA.Snapshot, acceptedB.Revision)
	if err != nil {
		report.Convergence.Detail = err.Error()
		return report
	}
	finalB, err := networkB.Snapshot(ctx)
	if err != nil {
		report.Convergence.Detail = err.Error()
		return report
	}
	report.ClientAHash, err = finalA.Hash()
	if err != nil {
		report.Convergence.Detail = err.Error()
		return report
	}
	report.ClientBHash, err = finalB.Hash()
	if err != nil {
		report.Convergence.Detail = err.Error()
		return report
	}
	if report.ClientAHash != report.ClientBHash {
		report.Convergence.Detail = "client map hashes differ"
		return report
	}
	report.Convergence.OK = true
	return report
}

func createSmokeJoinToken(ctx context.Context, baseURL, sessionID, ownerToken string) (string, error) {
	body, err := json.Marshal(map[string]string{"role": string(server.RoleEditor), "display_name": "client-b"})
	if err != nil {
		return "", err
	}
	requestURL := baseURL + "/v1/sessions/" + url.PathEscape(sessionID) + "/join-tokens"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+ownerToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create smoke join token returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("create smoke join token returned an empty token")
	}
	return result.Token, nil
}

func smokeOperation(snapshot model.Snapshot, coord model.Coord) (model.Operation, error) {
	operationIDs := map[int]model.OperationID{
		1: "01890f3e-7b5c-7abc-8def-012345678901",
		2: "01890f3e-7b5c-7abc-8def-012345678902",
	}
	stableIDs := map[int]model.StableID{
		1: "01890f3e-7b5c-7abc-8def-012345678911",
		2: "01890f3e-7b5c-7abc-8def-012345678912",
	}
	operationID, operationExists := operationIDs[coord.X]
	stableID, stableExists := stableIDs[coord.X]
	if !operationExists || !stableExists {
		return model.Operation{}, fmt.Errorf("smoke operation coordinate %d has no fixed identity", coord.X)
	}
	baseHash, err := snapshot.Hash()
	if err != nil {
		return model.Operation{}, err
	}
	before := model.TileState{}
	for _, tile := range snapshot.Tiles {
		if tile.Coord == coord {
			before = model.CloneTileState(tile.State)
			break
		}
	}
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		OperationID:     operationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     baseHash,
		Kind:            model.OperationKindTileChange,
		Changes: []model.TileChange{{
			Coord:  coord,
			Before: before,
			After:  model.TileState{Prefabs: []model.PrefabState{{StableID: stableID, Path: "/turf", Vars: map[string]string{}}}},
		}},
	}, nil
}

func waitForRevision(ctx context.Context, snapshot func(context.Context) (model.Snapshot, error), revision model.Revision) (model.Snapshot, error) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := snapshot(ctx)
		if err != nil {
			return model.Snapshot{}, err
		}
		if current.Revision >= revision {
			return current, nil
		}
		select {
		case <-ctx.Done():
			return model.Snapshot{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
