package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

const webSocketIOTimeout = 5 * time.Second

func (service *Service) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	if !service.originAllowed(request.Header.Get("Origin")) {
		writeError(writer, http.StatusForbidden, "origin_rejected", "WebSocket origin is not allowed")
		return
	}
	if !headerContains(request.Header.Values("Sec-WebSocket-Protocol"), WebSocketSubprotocol) {
		writeError(writer, http.StatusBadRequest, "protocol_required", "WebSocket protocol version is required")
		return
	}
	service.mutex.RLock()
	tokenValue := bearerToken(request)
	auth, authenticated := service.tokens[tokenValue]
	service.mutex.RUnlock()
	if !authenticated || auth.launch || !service.config.Now().Before(auth.expiresAt) {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "session token is invalid")
		return
	}

	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		Subprotocols:    []string{WebSocketSubprotocol},
		CompressionMode: websocket.CompressionDisabled,
		// Origin was matched exactly against the configured allowlist before the upgrade.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	connection.SetReadLimit(protocol.MaxMessageBytes)
	defer func() { _ = connection.CloseNow() }()
	// Accept hijacks the HTTP connection, so the service context owns the WebSocket lifetime.
	if err := service.serveWebSocket(service.context, connection, tokenValue, auth); err != nil && service.config.OnWebSocketError != nil {
		service.config.OnWebSocketError(err)
	}
}

func (service *Service) serveWebSocket(parent context.Context, connection *websocket.Conn, tokenValue string, auth tokenRecord) error {
	parent, cancelConnection := context.WithCancel(parent)
	defer cancelConnection()
	readContext, cancelRead := context.WithTimeout(parent, webSocketIOTimeout)
	_, data, err := connection.Read(readContext)
	cancelRead()
	if err != nil {
		return fmt.Errorf("read join: %w", err)
	}
	decoded, err := protocol.DecodeClient(data)
	if err != nil || decoded.Envelope.Type != protocol.ClientJoin || decoded.Envelope.SessionID != auth.sessionID {
		_ = connection.Close(websocket.StatusPolicyViolation, "valid join required")
		return fmt.Errorf("validate join envelope: %w", err)
	}
	join := decoded.Payload.(*protocol.JoinPayload)
	if join.JoinToken != tokenValue {
		_ = connection.Close(websocket.StatusPolicyViolation, "join token mismatch")
		return fmt.Errorf("validate join token: mismatch")
	}

	service.mutex.RLock()
	session, exists := service.sessions[auth.sessionID]
	service.mutex.RUnlock()
	if !exists {
		_ = connection.Close(websocket.StatusPolicyViolation, "session unavailable")
		return fmt.Errorf("load joined session: unavailable")
	}
	durable, cancelDurable, err := service.hub.SubscribeDurable(auth.sessionID, 64)
	if err != nil {
		return fmt.Errorf("subscribe durable operations: %w", err)
	}
	defer cancelDurable()
	presenceSnapshot, presenceUpdates, cancelPresence, err := service.hub.SubscribePresence(auth.sessionID, 1)
	if err != nil {
		return fmt.Errorf("subscribe presence: %w", err)
	}
	defer cancelPresence()
	defer service.hub.DisconnectPresence(auth.sessionID, auth.principal.ActorID())
	snapshot, err := session.owner.Snapshot(parent)
	if err != nil {
		return fmt.Errorf("load joined snapshot: %w", err)
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return fmt.Errorf("hash joined snapshot: %w", err)
	}
	if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{
		ProtocolVersion: model.ProtocolVersion,
		MessageID:       "joined-" + decoded.Envelope.MessageID,
		SessionID:       auth.sessionID,
		Type:            protocol.ServerJoined,
	}, protocol.JoinedPayload{DocumentID: snapshot.DocumentID, ActorID: auth.principal.ActorID(), Role: string(auth.principal.Role()), Revision: snapshot.Revision, MapHash: mapHash}); err != nil {
		return fmt.Errorf("write joined message: %w", err)
	}
	_, replay, err := service.store.Load(parent, snapshot.DocumentID)
	if err != nil {
		return fmt.Errorf("load reconnect replay: %w", err)
	}
	for _, accepted := range replay {
		if accepted.Revision <= join.AcknowledgedRevision {
			continue
		}
		acceptedHash, found, hashErr := service.store.RevisionHash(parent, snapshot.DocumentID, accepted.Revision)
		if hashErr != nil {
			return fmt.Errorf("load replay hash at revision %d: %w", accepted.Revision, hashErr)
		}
		if !found {
			return fmt.Errorf("replay hash at revision %d is not retained", accepted.Revision)
		}
		if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "replay-" + string(accepted.OperationID), SessionID: auth.sessionID, Type: protocol.ServerOperationAccepted}, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: acceptedHash}); err != nil {
			return fmt.Errorf("write replay operation: %w", err)
		}
	}
	if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "replay-complete-" + decoded.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerReplayComplete}, protocol.ReplayCompletePayload{Revision: snapshot.Revision, MapHash: mapHash}); err != nil {
		return fmt.Errorf("write replay completion: %w", err)
	}
	participants := make([]protocol.ParticipantPresence, 0, len(presenceSnapshot))
	for _, presence := range presenceSnapshot {
		participants = append(participants, protocol.ParticipantPresence{ActorID: presence.ActorID, DisplayName: presence.DisplayName, Sequence: presence.Sequence, Cursor: presence.Cursor, Status: presence.Status})
	}
	if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "presence-snapshot-" + decoded.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerPresenceSnapshot}, protocol.PresenceSnapshotPayload{Participants: participants}); err != nil {
		return fmt.Errorf("write presence snapshot: %w", err)
	}

	incoming := make(chan protocol.DecodedClient)
	readErrors := make(chan error, 1)
	go readClientMessages(parent, connection, auth.sessionID, incoming, readErrors)
	for {
		select {
		case <-parent.Done():
			return nil
		case err := <-readErrors:
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || errors.Is(err, context.Canceled) {
				return nil
			}
			return fmt.Errorf("read client message: %w", err)
		case accepted, open := <-durable:
			if !open {
				return fmt.Errorf("durable subscriber fell behind")
			}
			acceptedHash, found, hashErr := service.store.RevisionHash(parent, accepted.DocumentID, accepted.Revision)
			if hashErr != nil {
				return fmt.Errorf("load durable hash at revision %d: %w", accepted.Revision, hashErr)
			}
			if !found {
				return fmt.Errorf("durable hash at revision %d is not retained", accepted.Revision)
			}
			if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "accepted-" + string(accepted.OperationID), SessionID: auth.sessionID, Type: protocol.ServerOperationAccepted}, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: acceptedHash}); err != nil {
				return fmt.Errorf("write accepted operation: %w", err)
			}
		case presence, open := <-presenceUpdates:
			if !open {
				return nil
			}
			payload := protocol.ServerPresenceUpdatePayload{ActorID: presence.ActorID, DisplayName: presence.DisplayName, Sequence: presence.Sequence, Cursor: presence.Cursor, Status: presence.Status}
			if err := writeServerEnvelope(parent, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: fmt.Sprintf("presence-%s-%d", presence.ActorID, presence.Sequence), SessionID: auth.sessionID, Type: protocol.ServerPresenceUpdate}, payload); err != nil {
				return fmt.Errorf("write presence update: %w", err)
			}
		case message := <-incoming:
			if err := service.handleClientMessage(parent, connection, session, auth, message); err != nil {
				return err
			}
		}
	}
}

func readClientMessages(ctx context.Context, connection *websocket.Conn, sessionID string, messages chan<- protocol.DecodedClient, readErrors chan<- error) {
	for {
		readContext, cancel := context.WithTimeout(ctx, 2*webSocketIOTimeout)
		_, data, err := connection.Read(readContext)
		cancel()
		if err != nil {
			readErrors <- err
			return
		}
		message, err := protocol.DecodeClient(data)
		if err != nil || message.Envelope.SessionID != sessionID {
			if err == nil {
				err = fmt.Errorf("message session id does not match authenticated session")
			}
			readErrors <- err
			return
		}
		select {
		case messages <- message:
		case <-ctx.Done():
			return
		}
	}
}

func (service *Service) handleClientMessage(ctx context.Context, connection *websocket.Conn, session sessionRecord, auth tokenRecord, message protocol.DecodedClient) error {
	switch message.Envelope.Type {
	case protocol.ClientPing:
		ping := message.Payload.(*protocol.PingPayload)
		return writeServerEnvelope(ctx, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "pong-" + message.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerPong}, protocol.PongPayload{Nonce: ping.Nonce})
	case protocol.ClientPresenceUpdate:
		presence := message.Payload.(*protocol.PresenceUpdatePayload)
		if err := service.hub.UpdatePresence(auth.sessionID, auth.principal, PresenceUpdate{Sequence: presence.Sequence, Cursor: presence.Cursor, Status: presence.Status}); err != nil {
			return fmt.Errorf("update presence: %w", err)
		}
		return nil
	case protocol.ClientOperationSubmit:
		submission := message.Payload.(*protocol.OperationSubmitPayload)
		accepted, duplicate, err := service.hub.SubmitWithStatus(ctx, auth.sessionID, auth.principal, submission.Operation)
		if err != nil {
			current, snapshotErr := session.owner.Snapshot(ctx)
			if snapshotErr != nil {
				return fmt.Errorf("load rejected snapshot: %w", snapshotErr)
			}
			currentHash, _ := current.Hash()
			return writeServerEnvelope(ctx, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "rejected-" + message.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerOperationRejected}, protocol.OperationRejectedPayload{OperationID: submission.Operation.OperationID, Code: rejectionCode(err, "operation_rejected"), Message: "operation was rejected", Revision: current.Revision, MapHash: currentHash, AuthoritativeValues: authoritativeValues(current, submission.Operation.Changes)})
		}
		if duplicate {
			acceptedHash, found, hashErr := service.store.RevisionHash(ctx, accepted.DocumentID, accepted.Revision)
			if hashErr != nil {
				return fmt.Errorf("load duplicate hash: %w", hashErr)
			}
			if !found {
				return fmt.Errorf("duplicate hash at revision %d is not retained", accepted.Revision)
			}
			return writeServerEnvelope(ctx, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "duplicate-" + message.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerOperationAccepted}, protocol.OperationAcceptedPayload{Operation: accepted, MapHash: acceptedHash})
		}
		return nil
	case protocol.ClientAcknowledgedRevision:
		return nil
	case protocol.ClientInverseRequest:
		inverse := message.Payload.(*protocol.InverseRequestPayload)
		if _, err := service.hub.Inverse(ctx, auth.sessionID, auth.principal, inverse.OperationID); err != nil {
			current, snapshotErr := session.owner.Snapshot(ctx)
			if snapshotErr != nil {
				return fmt.Errorf("load inverse rejection snapshot: %w", snapshotErr)
			}
			currentHash, _ := current.Hash()
			var changes []model.TileChange
			if target, found, lookupErr := service.store.LookupOperation(ctx, current.DocumentID, inverse.OperationID); lookupErr == nil && found {
				changes = target.Changes
			}
			return writeServerEnvelope(ctx, connection, protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "inverse-rejected-" + message.Envelope.MessageID, SessionID: auth.sessionID, Type: protocol.ServerOperationRejected}, protocol.OperationRejectedPayload{OperationID: inverse.OperationID, Code: rejectionCode(err, "inverse_rejected"), Message: "inverse was rejected", Revision: current.Revision, MapHash: currentHash, AuthoritativeValues: authoritativeValues(current, changes)})
		}
		return nil
	default:
		return fmt.Errorf("unsupported client message %q", message.Envelope.Type)
	}
}

func rejectionCode(err error, fallback string) string {
	if code := engine.CodeOf(err); code != "" {
		return string(code)
	}
	return fallback
}

func authoritativeValues(snapshot model.Snapshot, changes []model.TileChange) []model.Tile {
	states := make(map[model.Coord]model.TileState, len(snapshot.Tiles))
	for _, tile := range snapshot.Tiles {
		states[tile.Coord] = tile.State
	}
	seen := make(map[model.Coord]struct{}, len(changes))
	values := make([]model.Tile, 0, len(changes))
	for _, change := range changes {
		if _, exists := seen[change.Coord]; exists {
			continue
		}
		seen[change.Coord] = struct{}{}
		values = append(values, model.Tile{Coord: change.Coord, State: model.CloneTileState(states[change.Coord])})
	}
	return values
}

func (service *Service) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range service.config.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func headerContains(values []string, target string) bool {
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if strings.TrimSpace(item) == target {
				return true
			}
		}
	}
	return false
}

func writeServerEnvelope(ctx context.Context, connection *websocket.Conn, envelope protocol.ServerEnvelope, payload any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	envelope.Payload = payloadJSON
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	writeContext, cancel := context.WithTimeout(ctx, webSocketIOTimeout)
	defer cancel()
	return connection.Write(writeContext, websocket.MessageText, data)
}
