package load

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

type RunConfig struct {
	Endpoint   string
	Origin     string
	SessionID  string
	OwnerToken string
}

type Result struct {
	Clients                  int            `json:"clients"`
	AcceptedOperations       int            `json:"accepted_operations"`
	PresenceUpdatesSent      int            `json:"presence_updates_sent"`
	FinalRevision            model.Revision `json:"final_revision"`
	FinalMapHash             string         `json:"final_map_hash"`
	ExpectedMapHash          string         `json:"expected_map_hash"`
	P50AcknowledgementMillis float64        `json:"p50_acknowledgement_ms"`
	P95AcknowledgementMillis float64        `json:"p95_acknowledgement_ms"`
	P99AcknowledgementMillis float64        `json:"p99_acknowledgement_ms"`
	Diverged                 bool           `json:"diverged"`
	GatePassed               bool           `json:"gate_passed"`
}

type acceptedEvent struct {
	client      int
	operationID model.OperationID
}

func Run(ctx context.Context, config RunConfig, scenario Scenario) (Result, error) {
	if err := validateRunConfig(config); err != nil {
		return Result{}, err
	}
	if err := scenario.Verify(); err != nil {
		return Result{}, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	tokens := make([]string, len(scenario.Actors))
	for index := range tokens {
		token, err := mintEditorToken(ctx, client, config, index)
		if err != nil {
			return Result{}, err
		}
		tokens[index] = token
	}
	connections := make([]*websocket.Conn, 0, len(tokens))
	defer func() {
		for _, connection := range connections {
			_ = connection.Close(websocket.StatusNormalClosure, "load scenario complete")
		}
	}()
	accepted := make(chan acceptedEvent, len(tokens)*2)
	readErrors := make(chan error, len(tokens))
	for index, token := range tokens {
		connection, err := connectEditor(ctx, config, token)
		if err != nil {
			return Result{}, err
		}
		connections = append(connections, connection)
		go readLoadEvents(ctx, index, connection, accepted, readErrors)
	}
	latencies := make([]time.Duration, 0, len(scenario.Operations))
	for index, operation := range scenario.Operations {
		if index > 0 && scenario.Config.TargetOperationsPerSecond > 0 {
			if err := waitContext(ctx, time.Second/time.Duration(scenario.Config.TargetOperationsPerSecond)); err != nil {
				return Result{}, err
			}
		}
		started := time.Now()
		if err := writeClientEnvelope(ctx, connections[index%len(connections)], protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "operation-" + string(operation.OperationID), SessionID: config.SessionID, Type: protocol.ClientOperationSubmit}, protocol.OperationSubmitPayload{Operation: operation}); err != nil {
			return Result{}, err
		}
		seen := make(map[int]struct{}, len(connections))
		for len(seen) < len(connections) {
			select {
			case event := <-accepted:
				if event.operationID == operation.OperationID {
					seen[event.client] = struct{}{}
				}
			case err := <-readErrors:
				return Result{}, err
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		latencies = append(latencies, time.Since(started))
	}
	actorConnections := make(map[model.ActorID]*websocket.Conn, len(scenario.Actors))
	for index, actorID := range scenario.Actors {
		actorConnections[actorID] = connections[index]
	}
	for index, presence := range scenario.Presence {
		if index > 0 && index%len(connections) == 0 && scenario.Config.TargetPresencePerSecondPerEditor > 0 {
			if err := waitContext(ctx, time.Second/time.Duration(scenario.Config.TargetPresencePerSecondPerEditor)); err != nil {
				return Result{}, err
			}
		}
		connection := actorConnections[presence.ActorID]
		if connection == nil {
			return Result{}, fmt.Errorf("presence actor %q has no load connection", presence.ActorID)
		}
		if err := writeClientEnvelope(ctx, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: fmt.Sprintf("presence-%d", index), SessionID: config.SessionID, Type: protocol.ClientPresenceUpdate}, protocol.PresenceUpdatePayload{Sequence: presence.Sequence, Cursor: &presence.Cursor, Status: "active"}); err != nil {
			return Result{}, err
		}
	}
	snapshot, err := fetchSnapshot(ctx, client, config, tokens[0])
	if err != nil {
		return Result{}, err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return Result{}, err
	}
	slices.Sort(latencies)
	result := Result{
		Clients: len(connections), AcceptedOperations: len(scenario.Operations), PresenceUpdatesSent: len(scenario.Presence), FinalRevision: snapshot.Revision,
		FinalMapHash: mapHash, ExpectedMapHash: scenario.ExpectedMapHash, P50AcknowledgementMillis: percentileMillis(latencies, 0.50), P95AcknowledgementMillis: percentileMillis(latencies, 0.95), P99AcknowledgementMillis: percentileMillis(latencies, 0.99),
	}
	result.Diverged = result.FinalRevision != scenario.ExpectedRevision || result.FinalMapHash != scenario.ExpectedMapHash
	result.GatePassed = !result.Diverged && result.AcceptedOperations == len(scenario.Operations) && (scenario.Config.MaximumP95AcknowledgementMilliseconds == 0 || result.P95AcknowledgementMillis <= float64(scenario.Config.MaximumP95AcknowledgementMilliseconds))
	return result, nil
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func validateRunConfig(config RunConfig) error {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && strings.HasPrefix(endpoint.Hostname(), "127."))) || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("load endpoint must be HTTPS or loopback HTTP")
	}
	if config.Origin == "" || config.SessionID == "" || len(config.SessionID) > protocol.MaxIdentifierBytes || config.OwnerToken == "" {
		return fmt.Errorf("load origin, session ID, and owner token are required")
	}
	return nil
}

func mintEditorToken(ctx context.Context, client *http.Client, config RunConfig, index int) (string, error) {
	body, _ := json.Marshal(map[string]any{"role": "editor", "display_name": fmt.Sprintf("Load Editor %d", index+1)})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.Endpoint, "/")+"/v1/sessions/"+url.PathEscape(config.SessionID)+"/join-tokens", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+config.OwnerToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("mint editor token returned HTTP %d", response.StatusCode)
	}
	var value struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return "", fmt.Errorf("decode editor token: %w", err)
	}
	if value.Token == "" {
		return "", fmt.Errorf("decode editor token: response token is empty")
	}
	return value.Token, nil
}

func connectEditor(ctx context.Context, config RunConfig, token string) (*websocket.Conn, error) {
	websocketURL := "ws" + strings.TrimPrefix(strings.TrimRight(config.Endpoint, "/"), "http") + "/v1/collaboration"
	connection, _, err := websocket.Dial(ctx, websocketURL, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}, "Origin": []string{config.Origin}}, Subprotocols: []string{"apheliondmm.collaboration.v1"}})
	if err != nil {
		return nil, err
	}
	if err := writeClientEnvelope(ctx, connection, protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "join", SessionID: config.SessionID, Type: protocol.ClientJoin}, protocol.JoinPayload{JoinToken: token}); err != nil {
		_ = connection.CloseNow()
		return nil, err
	}
	joined, replayed, presence := false, false, false
	for !joined || !replayed || !presence {
		_, data, err := connection.Read(ctx)
		if err != nil {
			_ = connection.CloseNow()
			return nil, err
		}
		message, err := protocol.DecodeServer(data)
		if err != nil {
			_ = connection.CloseNow()
			return nil, err
		}
		switch message.Envelope.Type {
		case protocol.ServerJoined:
			joined = true
		case protocol.ServerReplayComplete:
			replayed = true
		case protocol.ServerPresenceSnapshot:
			presence = true
		}
	}
	return connection, nil
}

func readLoadEvents(ctx context.Context, client int, connection *websocket.Conn, accepted chan<- acceptedEvent, readErrors chan<- error) {
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			if ctx.Err() == nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				readErrors <- err
			}
			return
		}
		message, err := protocol.DecodeServer(data)
		if err != nil {
			readErrors <- err
			return
		}
		if message.Envelope.Type == protocol.ServerOperationAccepted {
			operation := message.Payload.(*protocol.OperationAcceptedPayload).Operation
			accepted <- acceptedEvent{client: client, operationID: operation.OperationID}
		}
	}
}

func fetchSnapshot(ctx context.Context, client *http.Client, config RunConfig, token string) (model.Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(config.Endpoint, "/")+"/v1/sessions/"+url.PathEscape(config.SessionID)+"/snapshot", nil)
	if err != nil {
		return model.Snapshot{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return model.Snapshot{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return model.Snapshot{}, fmt.Errorf("fetch final snapshot returned HTTP %d", response.StatusCode)
	}
	var snapshot model.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func writeClientEnvelope(ctx context.Context, connection *websocket.Conn, envelope protocol.ClientEnvelope, payload any) error {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	envelope.Payload = encodedPayload
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return connection.Write(ctx, websocket.MessageText, data)
}

func percentileMillis(values []time.Duration, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * percentile)
	return float64(values[index]) / float64(time.Millisecond)
}
