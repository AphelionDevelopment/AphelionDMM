package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	collabclient "sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

const maxSessionResponseBytes = 256 << 20

type SessionClientConfig struct {
	HTTPTimeout time.Duration
	Transport   collabclient.TransportConfig
	Now         func() time.Time
}

type SessionClient struct {
	config SessionClientConfig
	http   *http.Client

	mutex        sync.Mutex
	joining      bool
	transport    *collabclient.WebSocketTransport
	network      *collabclient.NetworkExecutor
	machine      *collabclient.StateMachine
	sessionID    string
	role         string
	revision     model.Revision
	participants map[model.ActorID]ObservedPresence
	lastErr      error
}

func NewSessionClient(config SessionClientConfig) *SessionClient {
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = 10 * time.Second
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &SessionClient{config: config, machine: collabclient.NewStateMachine(), participants: make(map[model.ActorID]ObservedPresence), http: &http.Client{Timeout: config.HTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}}
}

func (client *SessionClient) Create(ctx context.Context, baseURL, launchToken string, snapshot model.Snapshot) (Invitation, error) {
	if launchToken == "" {
		return Invitation{}, fmt.Errorf("collaboration launch token is empty")
	}
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		return Invitation{}, err
	}
	request, err := client.request(ctx, http.MethodPost, baseURL+"/v1/sessions", launchToken, bytes.NewReader(body))
	if err != nil {
		return Invitation{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return Invitation{}, fmt.Errorf("create collaboration session: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		return Invitation{}, fmt.Errorf("create collaboration session returned HTTP %d", response.StatusCode)
	}
	var created struct {
		SessionID           string           `json:"session_id"`
		DocumentID          model.DocumentID `json:"document_id"`
		Revision            model.Revision   `json:"revision"`
		MapHash             string           `json:"map_hash"`
		OwnerToken          string           `json:"owner_token"`
		OwnerTokenExpiresAt time.Time        `json:"owner_token_expires_at"`
	}
	if err := decodeLimited(response.Body, &created); err != nil {
		return Invitation{}, fmt.Errorf("decode collaboration session: %w", err)
	}
	if created.SessionID == "" || created.OwnerToken == "" || created.DocumentID != snapshot.DocumentID {
		return Invitation{}, fmt.Errorf("collaboration session response is incompatible with the requested document")
	}
	return Invitation{BaseURL: strings.TrimRight(baseURL, "/"), Origin: strings.TrimRight(baseURL, "/"), SessionID: created.SessionID, Token: created.OwnerToken}, nil
}

func (client *SessionClient) Join(ctx context.Context, invitation Invitation) error {
	if err := invitation.validate(); err != nil {
		return err
	}
	client.mutex.Lock()
	if client.joining || client.transport != nil {
		client.mutex.Unlock()
		return ErrSessionActive
	}
	client.joining = true
	client.machine = collabclient.NewStateMachine()
	_ = client.machine.Apply(collabclient.EventConnect)
	client.sessionID = invitation.SessionID
	client.role = ""
	client.revision = 0
	client.participants = make(map[model.ActorID]ObservedPresence)
	client.lastErr = nil
	machine := client.machine
	client.mutex.Unlock()
	joined := false
	defer func() {
		if !joined {
			client.mutex.Lock()
			client.joining = false
			client.mutex.Unlock()
		}
	}()

	snapshot, err := client.fetchSnapshot(ctx, invitation)
	if err != nil {
		client.recordConnectionFailure(machine, err)
		return err
	}
	transport := collabclient.NewWebSocketTransport(client.config.Transport)
	var routeMutex sync.Mutex
	var network *collabclient.NetworkExecutor
	ready := make(chan struct{}, 1)
	errorsFound := make(chan error, 1)
	receive := func(envelope protocol.ServerEnvelope) {
		decoded, decodeErr := decodeServerEnvelope(envelope)
		if decodeErr != nil {
			nonBlockingError(errorsFound, decodeErr)
			return
		}
		routeMutex.Lock()
		defer routeMutex.Unlock()
		switch decoded.Envelope.Type {
		case protocol.ServerJoined:
			payload := decoded.Payload.(*protocol.JoinedPayload)
			if payload.DocumentID != snapshot.DocumentID {
				nonBlockingError(errorsFound, fmt.Errorf("joined document does not match fetched snapshot"))
				return
			}
			network, decodeErr = collabclient.NewNetworkExecutor(transport, snapshot, payload.ActorID, invitation.SessionID)
			if decodeErr != nil {
				nonBlockingError(errorsFound, decodeErr)
				return
			}
			client.recordJoined(machine, payload)
		case protocol.ServerOperationAccepted, protocol.ServerOperationRejected:
			if network == nil {
				nonBlockingError(errorsFound, fmt.Errorf("received operation before joined message"))
				return
			}
			network.Receive(envelope)
			client.recordOperation(machine, network, decoded.Envelope.Type)
		case protocol.ServerReplayComplete:
			if network == nil {
				nonBlockingError(errorsFound, fmt.Errorf("received replay completion before joined message"))
				return
			}
			payload := decoded.Payload.(*protocol.ReplayCompletePayload)
			current, snapshotErr := network.Snapshot(context.Background())
			currentHash, hashErr := current.Hash()
			if snapshotErr != nil || hashErr != nil || current.Revision != payload.Revision || currentHash != payload.MapHash {
				nonBlockingError(errorsFound, fmt.Errorf("replay completion does not match client revision"))
				return
			}
			client.recordSynchronized(machine, payload.Revision)
			select {
			case ready <- struct{}{}:
			default:
			}
		case protocol.ServerPresenceSnapshot:
			client.recordPresenceSnapshot(decoded.Payload.(*protocol.PresenceSnapshotPayload))
		case protocol.ServerPresenceUpdate:
			client.recordPresenceUpdate(decoded.Payload.(*protocol.ServerPresenceUpdatePayload))
		}
	}
	if err := transport.Connect(ctx, protocol.JoinRequest{BaseURL: invitation.BaseURL, Origin: invitation.Origin, Token: invitation.Token, SessionID: invitation.SessionID, AcknowledgedRevision: snapshot.Revision}, receive); err != nil {
		client.recordConnectionFailure(machine, err)
		return err
	}
	invitation.Token = ""
	select {
	case <-ready:
	case joinErr := <-errorsFound:
		_ = transport.Close(websocket.StatusPolicyViolation, "join failed")
		client.recordConnectionFailure(machine, joinErr)
		return joinErr
	case <-ctx.Done():
		_ = transport.Close(websocket.StatusGoingAway, "join canceled")
		client.recordConnectionFailure(machine, ctx.Err())
		return ctx.Err()
	}
	routeMutex.Lock()
	joinedNetwork := network
	routeMutex.Unlock()
	if joinedNetwork == nil {
		_ = transport.Close(websocket.StatusInternalError, "join incomplete")
		return fmt.Errorf("collaboration join completed without an executor")
	}
	client.mutex.Lock()
	client.transport = transport
	client.network = joinedNetwork
	client.joining = false
	client.mutex.Unlock()
	joined = true
	go client.monitorTransport(machine, transport, joinedNetwork)
	return nil
}

func (client *SessionClient) Leave(context.Context) error {
	client.mutex.Lock()
	transport := client.transport
	network := client.network
	client.transport = nil
	client.network = nil
	client.joining = false
	machine := client.machine
	client.participants = make(map[model.ActorID]ObservedPresence)
	client.mutex.Unlock()
	if machine != nil && machine.State() != collabclient.StateClosed {
		_ = machine.Apply(collabclient.EventClose)
	}
	if network != nil {
		network.Terminate(collabclient.ErrExecutorTerminated)
	}
	if transport == nil {
		return nil
	}
	return transport.Close(websocket.StatusNormalClosure, "left collaboration session")
}

func (client *SessionClient) HasUnacknowledgedOperations() bool {
	client.mutex.Lock()
	network := client.network
	client.mutex.Unlock()
	return network != nil && network.HasUnacknowledgedOperations()
}

func (client *SessionClient) NetworkExecutor() *collabclient.NetworkExecutor {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return client.network
}

func (client *SessionClient) CollaborationExecutor() executor.Executor {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return client.network
}

func (client *SessionClient) Status() SessionStatus {
	client.mutex.Lock()
	machine := client.machine
	status := SessionStatus{SessionID: client.sessionID, Role: client.role, Revision: client.revision, Err: client.lastErr}
	status.Participants = make([]protocol.ParticipantPresence, 0, len(client.participants))
	for _, observed := range client.participants {
		status.Participants = append(status.Participants, observed.Presence)
	}
	network := client.network
	client.mutex.Unlock()
	if machine == nil {
		status.State = collabclient.StateDisconnected
	} else {
		status.State = machine.State()
	}
	if network != nil {
		status.Conflicts = network.Conflicts()
	}
	return status
}

func (client *SessionClient) ObservedPresence() []ObservedPresence {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	result := make([]ObservedPresence, 0, len(client.participants))
	for _, observed := range client.participants {
		copy := observed
		if observed.Presence.Cursor != nil {
			cursor := *observed.Presence.Cursor
			copy.Presence.Cursor = &cursor
		}
		result = append(result, copy)
	}
	return result
}

func (client *SessionClient) recordJoined(machine *collabclient.StateMachine, payload *protocol.JoinedPayload) {
	if machine.State() == collabclient.StateConnecting {
		_ = machine.Apply(collabclient.EventConnected)
	}
	client.mutex.Lock()
	if client.machine == machine {
		client.role = payload.Role
		client.revision = payload.Revision
	}
	client.mutex.Unlock()
}

func (client *SessionClient) recordSynchronized(machine *collabclient.StateMachine, revision model.Revision) {
	if machine.State() == collabclient.StateConnecting {
		_ = machine.Apply(collabclient.EventConnected)
	}
	if machine.State() == collabclient.StateSynchronizing {
		_ = machine.Apply(collabclient.EventSynchronized)
	}
	client.mutex.Lock()
	if client.machine == machine {
		client.revision = revision
	}
	client.mutex.Unlock()
}

func (client *SessionClient) recordOperation(machine *collabclient.StateMachine, network *collabclient.NetworkExecutor, messageType protocol.ServerType) {
	snapshot, err := network.Snapshot(context.Background())
	client.mutex.Lock()
	if client.machine == machine {
		if err != nil {
			client.lastErr = err
		} else {
			client.revision = snapshot.Revision
		}
	}
	client.mutex.Unlock()
	if messageType == protocol.ServerOperationRejected && (machine.State() == collabclient.StateSynchronizing || machine.State() == collabclient.StateCaughtUp) {
		_ = machine.Apply(collabclient.EventConflict)
	}
}

func (client *SessionClient) recordPresenceSnapshot(payload *protocol.PresenceSnapshotPayload) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.participants = make(map[model.ActorID]ObservedPresence, len(payload.Participants))
	for _, participant := range payload.Participants {
		client.participants[participant.ActorID] = ObservedPresence{Presence: cloneParticipantPresence(participant), ObservedAt: client.config.Now()}
	}
}

func (client *SessionClient) recordPresenceUpdate(payload *protocol.ServerPresenceUpdatePayload) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	participant := protocol.ParticipantPresence{ActorID: payload.ActorID, DisplayName: payload.DisplayName, Sequence: payload.Sequence, Cursor: payload.Cursor, Status: payload.Status}
	client.participants[payload.ActorID] = ObservedPresence{Presence: cloneParticipantPresence(participant), ObservedAt: client.config.Now()}
}

func (client *SessionClient) recordConnectionFailure(machine *collabclient.StateMachine, err error) {
	if machine.State() == collabclient.StateClosed {
		return
	}
	switch machine.State() {
	case collabclient.StateConnecting, collabclient.StateSynchronizing, collabclient.StateCaughtUp, collabclient.StateReadOnly, collabclient.StateConflict:
		_ = machine.Apply(collabclient.EventConnectionLost)
	}
	client.mutex.Lock()
	if client.machine == machine {
		client.lastErr = err
		client.participants = make(map[model.ActorID]ObservedPresence)
	}
	client.mutex.Unlock()
}

func (client *SessionClient) monitorTransport(machine *collabclient.StateMachine, transport *collabclient.WebSocketTransport, network *collabclient.NetworkExecutor) {
	err := transport.Wait(context.Background())
	network.Terminate(err)
	client.recordConnectionFailure(machine, err)
}

func cloneParticipantPresence(participant protocol.ParticipantPresence) protocol.ParticipantPresence {
	if participant.Cursor != nil {
		cursor := *participant.Cursor
		participant.Cursor = &cursor
	}
	return participant
}

func (client *SessionClient) fetchSnapshot(ctx context.Context, invitation Invitation) (model.Snapshot, error) {
	path := strings.TrimRight(invitation.BaseURL, "/") + "/v1/sessions/" + url.PathEscape(invitation.SessionID) + "/snapshot"
	request, err := client.request(ctx, http.MethodGet, path, invitation.Token, nil)
	if err != nil {
		return model.Snapshot{}, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("fetch collaboration snapshot: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return model.Snapshot{}, fmt.Errorf("fetch collaboration snapshot returned HTTP %d", response.StatusCode)
	}
	var snapshot model.Snapshot
	if err := decodeLimited(response.Body, &snapshot); err != nil {
		return model.Snapshot{}, fmt.Errorf("decode collaboration snapshot: %w", err)
	}
	if _, err := snapshot.Hash(); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func (client *SessionClient) request(ctx context.Context, method, rawURL, token string, body io.Reader) (*http.Request, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("collaboration URL is invalid")
	}
	if parsed.Scheme == "http" {
		ip := net.ParseIP(parsed.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("cleartext collaboration URL is restricted to loopback IP addresses")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return request, nil
}

func decodeLimited(reader io.Reader, value any) error {
	decoder := json.NewDecoder(io.LimitReader(reader, maxSessionResponseBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("response contains trailing JSON")
	}
	return nil
}

func decodeServerEnvelope(envelope protocol.ServerEnvelope) (protocol.DecodedServer, error) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return protocol.DecodedServer{}, err
	}
	return protocol.DecodeServer(data)
}

func nonBlockingError(destination chan<- error, err error) {
	select {
	case destination <- err:
	default:
	}
}
