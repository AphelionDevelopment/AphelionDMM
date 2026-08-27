package relay

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/protocolv2"
)

const (
	WebSocketSubprotocol = "apheliondmm.relay.v2"
	connectionQueueDepth = 64
	initialFrameTimeout  = 10 * time.Second
)

var errClientDisconnect = errors.New("client requested disconnect")

type Service struct {
	config      Config
	registry    *Registry
	router      *Router
	mutex       sync.Mutex
	connections map[connectionKey]*peer
	active      int
	connects    *windowLimiter
	roomBytes   *windowLimiter
}

type connectionKey struct {
	room  protocolv2.RoomID
	actor protocolv2.ActorKey
}

type peer struct {
	outbound chan []byte
	cancel   context.CancelFunc
	messages *windowLimiter
}

func NewService(config Config) *Service {
	registry := NewRegistryWithLimits(config.RoomIdleTTL.Time(), config.Limits.MaxRooms, config.Limits.MaxConnectionsPerRoom)
	connectEntries := config.Limits.MaxConnections * 4
	if connectEntries < 1 {
		connectEntries = 1
	}
	roomEntries := config.Limits.MaxRooms
	if roomEntries < 1 {
		roomEntries = 1
	}
	return &Service{
		config: config, registry: registry, router: NewRouter(registry), connections: make(map[connectionKey]*peer),
		connects:  newWindowLimiter(int64(config.Limits.ConnectBurst), config.Limits.ConnectWindow.Time(), connectEntries),
		roomBytes: newWindowLimiter(config.Limits.MaxRoomBytesPerSecond, time.Second, roomEntries),
	}
}

func (service *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", service.handleHealth)
	mux.HandleFunc("GET /health/ready", service.handleHealth)
	mux.HandleFunc("GET /version", service.handleVersion)
	mux.HandleFunc("GET /v1/health/live", service.handleHealth)
	mux.HandleFunc("GET /v1/health/ready", service.handleHealth)
	mux.HandleFunc("GET /v1/version", service.handleVersion)
	mux.HandleFunc("GET /metrics", service.handleMetrics)
	mux.HandleFunc("GET /v2/collaboration", service.handleWebSocket)
	return mux
}

func (service *Service) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "protocol_versions": []uint16{protocolv2.Version}})
}

func (service *Service) handleVersion(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"service": "apheliondmm-relay", "protocol_versions": []uint16{protocolv2.Version}})
}

func (service *Service) handleMetrics(writer http.ResponseWriter, _ *http.Request) {
	service.mutex.Lock()
	active := service.active
	service.mutex.Unlock()
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, "apheliondmm_relay_active_connections %d\napheliondmm_relay_rooms %d\n", active, service.registry.RoomCount())
}

func (service *Service) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	if origin := request.Header.Get("Origin"); origin != "" && origin != service.config.PublicOrigin {
		http.Error(writer, "origin rejected", http.StatusForbidden)
		return
	}
	remoteHost, err := service.clientAddress(request)
	if err != nil {
		http.Error(writer, "remote address rejected", http.StatusBadRequest)
		return
	}
	if !service.connects.Allow(remoteHost, 1, time.Now().UTC()) {
		http.Error(writer, "connection rate limit reached", http.StatusTooManyRequests)
		return
	}
	if !service.acquire() {
		http.Error(writer, "connection limit reached", http.StatusTooManyRequests)
		return
	}
	defer service.release()
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{Subprotocols: []string{WebSocketSubprotocol}, CompressionMode: websocket.CompressionDisabled, InsecureSkipVerify: true})
	if err != nil {
		return
	}
	connection.SetReadLimit(int64(service.config.Limits.MaxMessageBytes + 256))
	defer func() { _ = connection.CloseNow() }()
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	readContext, cancelRead := context.WithTimeout(ctx, initialFrameTimeout)
	messageType, encoded, err := connection.Read(readContext)
	cancelRead()
	if err != nil || messageType != websocket.MessageBinary {
		_ = connection.Close(websocket.StatusPolicyViolation, "binary control frame required")
		return
	}
	envelope, err := protocolv2.UnmarshalEnvelope(encoded)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid protocol frame")
		return
	}
	validator := &protocolv2.SequenceValidator{}
	if err := validator.Accept(envelope.Header); err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid sequence")
		return
	}
	payload, err := protocolv2.VerifyControl(envelope)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "invalid control signature")
		return
	}
	if err := service.admit(envelope.Header, payload, time.Now().UTC()); err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "connection rejected")
		return
	}
	key := connectionKey{room: envelope.Header.RoomID, actor: envelope.Header.Sender}
	current := service.register(ctx, key, cancel)
	defer service.unregister(key, current, time.Now().UTC())
	if err := connection.Write(ctx, websocket.MessageBinary, encoded); err != nil {
		return
	}
	writeErrors := make(chan error, 1)
	go service.writeLoop(ctx, connection, current.outbound, writeErrors)
	for {
		messageType, encoded, err = connection.Read(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageBinary {
			_ = connection.Close(websocket.StatusUnsupportedData, "binary frames required")
			return
		}
		select {
		case <-writeErrors:
			return
		default:
		}
		if !current.messages.Allow("connection", 1, time.Now().UTC()) || !service.roomBytes.Allow(string(key.room[:]), int64(len(encoded)), time.Now().UTC()) {
			_ = connection.Close(websocket.StatusPolicyViolation, "relay rate limit reached")
			return
		}
		if err := service.handleFrame(ctx, key, validator, encoded); err != nil {
			if !errors.Is(err, errClientDisconnect) {
				_ = connection.Close(websocket.StatusPolicyViolation, "invalid relay frame")
			}
			return
		}
	}
}

func (service *Service) clientAddress(request *http.Request) (string, error) {
	remoteHost, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil || net.ParseIP(remoteHost) == nil {
		return "", fmt.Errorf("remote address is invalid")
	}
	trusted := false
	for _, encoded := range service.config.TrustedProxyCIDRs {
		_, network, parseErr := net.ParseCIDR(encoded)
		if parseErr == nil && network.Contains(net.ParseIP(remoteHost)) {
			trusted = true
			break
		}
	}
	if !trusted {
		return remoteHost, nil
	}
	forwarded := strings.TrimSpace(request.Header.Get("CF-Connecting-IP"))
	if forwarded == "" {
		forwardedFor := request.Header.Get("X-Forwarded-For")
		if forwardedFor == "" {
			return remoteHost, nil
		}
		forwarded = strings.TrimSpace(strings.Split(forwardedFor, ",")[0])
	}
	if net.ParseIP(forwarded) == nil {
		return "", fmt.Errorf("trusted proxy client address is invalid")
	}
	return forwarded, nil
}

func (service *Service) admit(header protocolv2.Header, payload any, now time.Time) error {
	switch header.Kind {
	case protocolv2.KindRoomCreate:
		if _, ok := payload.(*protocolv2.RoomCreateControl); !ok {
			return fmt.Errorf("invalid room creation")
		}
		return service.registry.CreateRoom(header.RoomID, header.Sender, now)
	case protocolv2.KindConnect:
		connect, ok := payload.(*protocolv2.ConnectControl)
		if !ok {
			return fmt.Errorf("invalid connection")
		}
		_, err := service.registry.Connect(header.RoomID, header.Sender, connect.Admission[:], now)
		return err
	default:
		return fmt.Errorf("first frame must create or connect to a room")
	}
}

func (service *Service) handleFrame(ctx context.Context, key connectionKey, validator *protocolv2.SequenceValidator, encoded []byte) error {
	envelope, err := protocolv2.UnmarshalEnvelope(encoded)
	if err != nil {
		return err
	}
	if envelope.Header.RoomID != key.room || envelope.Header.Sender != key.actor {
		return fmt.Errorf("frame identity changed")
	}
	if err := validator.Accept(envelope.Header); err != nil {
		return err
	}
	if envelope.Header.Route == protocolv2.RouteControl {
		payload, err := protocolv2.VerifyControl(envelope)
		if err != nil {
			return err
		}
		switch value := payload.(type) {
		case *protocolv2.AdmissionReplaceControl:
			admissions := make([]Admission, 0, len(value.Admissions))
			for _, admission := range value.Admissions {
				admissions = append(admissions, Admission{Digest: admission.Digest, Role: admission.Role, ExpiresAt: admission.ExpiresAt, BoundActor: admission.BoundActor})
			}
			return service.registry.ReplaceAdmissions(key.room, key.actor, value.Generation, admissions)
		case *protocolv2.OwnerTransferControl:
			return service.acceptOwnerTransfer(key.room, key.actor, value.Transfer, time.Now().UTC())
		case *protocolv2.HeartbeatControl:
			return nil
		case *protocolv2.DisconnectControl:
			return errClientDisconnect
		default:
			return fmt.Errorf("control message is not valid after admission")
		}
	}
	if err := protocolv2.VerifyEnvelopeSignature(envelope); err != nil {
		return err
	}
	targets, err := service.router.Targets(envelope)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := service.send(ctx, connectionKey{room: key.room, actor: target}, encoded); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) acceptOwnerTransfer(roomID protocolv2.RoomID, sender protocolv2.ActorKey, transfer protocolv2.OwnershipTransferred, now time.Time) error {
	room, found := service.registry.Snapshot(roomID)
	if !found || room.Owner != sender {
		return fmt.Errorf("ownership transfer sender is not the current owner")
	}
	manifest := transfer.Manifest
	if manifest.RoomID != roomID || manifest.Generation == 0 || manifest.OwnerKey == (protocolv2.ActorKey{}) {
		return fmt.Errorf("ownership transfer manifest is invalid")
	}
	seen := make(map[protocolv2.ActorKey]struct{}, len(manifest.Members))
	ownerCount := 0
	oldOwnerRole := protocolv2.Role("")
	newOwnerRole := protocolv2.Role("")
	for _, member := range manifest.Members {
		if member.ActorKey == (protocolv2.ActorKey{}) {
			return fmt.Errorf("ownership transfer manifest contains an empty actor")
		}
		if _, duplicate := seen[member.ActorKey]; duplicate {
			return fmt.Errorf("ownership transfer manifest contains a duplicate actor")
		}
		seen[member.ActorKey] = struct{}{}
		switch member.Role {
		case protocolv2.RoleOwner:
			ownerCount++
		case protocolv2.RoleEditor, protocolv2.RoleViewer:
		default:
			return fmt.Errorf("ownership transfer manifest contains an invalid role")
		}
		if member.ActorKey == sender {
			oldOwnerRole = member.Role
		}
		if member.ActorKey == manifest.OwnerKey {
			newOwnerRole = member.Role
		}
	}
	if ownerCount != 1 || oldOwnerRole != protocolv2.RoleEditor || newOwnerRole != protocolv2.RoleOwner || manifest.OwnerKey == sender {
		return fmt.Errorf("ownership transfer manifest does not describe a valid owner transition")
	}
	offer := protocolv2.OwnershipOffer{TargetActor: manifest.OwnerKey, NewOwnerKey: manifest.OwnerKey, Generation: manifest.Generation, Manifest: manifest}
	digest, err := protocolv2.DigestOwnershipOffer(offer)
	if err != nil {
		return err
	}
	if digest != transfer.OfferSHA256 {
		return fmt.Errorf("ownership transfer offer digest does not match the manifest")
	}
	if !ed25519.Verify(ed25519.PublicKey(sender[:]), digest[:], transfer.OldOwnerSignature[:]) {
		return fmt.Errorf("ownership transfer old owner signature is invalid")
	}
	if !ed25519.Verify(ed25519.PublicKey(manifest.OwnerKey[:]), digest[:], transfer.NewOwnerSignature[:]) {
		return fmt.Errorf("ownership transfer new owner signature is invalid")
	}
	return service.registry.TransferOwner(roomID, sender, manifest.OwnerKey, now)
}

func (service *Service) register(ctx context.Context, key connectionKey, cancel context.CancelFunc) *peer {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if prior := service.connections[key]; prior != nil {
		prior.cancel()
	}
	current := &peer{
		outbound: make(chan []byte, connectionQueueDepth),
		cancel:   cancel,
		messages: newWindowLimiter(int64(service.config.Limits.MessageBurst), service.config.Limits.MessageWindow.Time(), 1),
	}
	service.connections[key] = current
	return current
}

func (service *Service) unregister(key connectionKey, current *peer, now time.Time) {
	service.mutex.Lock()
	if service.connections[key] == current {
		delete(service.connections, key)
		service.registry.Disconnect(key.room, key.actor, now)
	}
	service.mutex.Unlock()
}

func (service *Service) send(ctx context.Context, key connectionKey, encoded []byte) error {
	service.mutex.Lock()
	target := service.connections[key]
	service.mutex.Unlock()
	if target == nil {
		return fmt.Errorf("relay target disconnected")
	}
	copyOfFrame := append([]byte(nil), encoded...)
	select {
	case target.outbound <- copyOfFrame:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		target.cancel()
		return fmt.Errorf("slow_consumer")
	}
}

func (service *Service) writeLoop(ctx context.Context, connection *websocket.Conn, outbound <-chan []byte, errors chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case encoded := <-outbound:
			if err := connection.Write(ctx, websocket.MessageBinary, encoded); err != nil {
				select {
				case errors <- err:
				default:
				}
				return
			}
		}
	}
}

func (service *Service) acquire() bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.active >= service.config.Limits.MaxConnections {
		return false
	}
	service.active++
	return true
}

func (service *Service) release() {
	service.mutex.Lock()
	service.active--
	service.mutex.Unlock()
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
