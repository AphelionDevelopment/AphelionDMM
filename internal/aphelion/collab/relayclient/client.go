package relayclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

type Client struct {
	connection   *websocket.Conn
	privateKey   ed25519.PrivateKey
	actorKey     protocolv2.ActorKey
	roomID       protocolv2.RoomID
	mutex        sync.Mutex
	sequence     uint64
	receiveMutex sync.Mutex
	validators   map[protocolv2.ActorKey]*protocolv2.SequenceValidator
	seenMessages map[protocolv2.MessageID]struct{}
	seenOrder    []protocolv2.MessageID
}

const inboundReplayCacheSize = 4096

func ConnectOwner(ctx context.Context, endpoint string, roomID protocolv2.RoomID, privateKey ed25519.PrivateKey) (*Client, error) {
	return connect(ctx, endpoint, roomID, privateKey, protocolv2.KindRoomCreate, protocolv2.RoomCreateControl{})
}

func ConnectParticipant(ctx context.Context, endpoint string, roomID protocolv2.RoomID, privateKey ed25519.PrivateKey, capability [32]byte) (*Client, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		client, err := connect(ctx, endpoint, roomID, privateKey, protocolv2.KindConnect, protocolv2.ConnectControl{Admission: capability})
		if err == nil {
			return client, nil
		}
		lastErr = err
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation || attempt == 4 {
			break
		}
		delay := 25 * time.Millisecond * time.Duration(1<<attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func connect(ctx context.Context, endpoint string, roomID protocolv2.RoomID, privateKey ed25519.PrivateKey, kind protocolv2.Kind, payload any) (*Client, error) {
	webSocketURL, _, err := relayWebSocketURL(endpoint)
	if err != nil {
		return nil, err
	}
	if roomID == (protocolv2.RoomID{}) || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("relay client identity is invalid")
	}
	var actorKey protocolv2.ActorKey
	copy(actorKey[:], privateKey.Public().(ed25519.PublicKey))
	header := protocolv2.Header{Version: protocolv2.Version, Route: protocolv2.RouteControl, Kind: kind, RoomID: roomID, Sender: actorKey, Sequence: 1}
	if _, err := rand.Read(header.MessageID[:]); err != nil {
		return nil, fmt.Errorf("generate relay message ID: %w", err)
	}
	envelope, err := protocolv2.SignControl(rand.Reader, privateKey, header, payload)
	if err != nil {
		return nil, err
	}
	encoded, err := protocolv2.MarshalEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	connection, _, err := websocket.Dial(ctx, webSocketURL, &websocket.DialOptions{Subprotocols: []string{relay.WebSocketSubprotocol}})
	if err != nil {
		return nil, fmt.Errorf("connect relay WebSocket: %w", err)
	}
	connection.SetReadLimit(protocolv2.MaxCiphertextBytes + 256)
	if err := connection.Write(ctx, websocket.MessageBinary, encoded); err != nil {
		_ = connection.CloseNow()
		return nil, fmt.Errorf("send relay admission: %w", err)
	}
	messageType, acknowledgement, err := connection.Read(ctx)
	if err != nil {
		_ = connection.CloseNow()
		return nil, fmt.Errorf("read relay admission acknowledgement: %w", err)
	}
	if messageType != websocket.MessageBinary || string(acknowledgement) != string(encoded) {
		_ = connection.CloseNow()
		return nil, fmt.Errorf("relay admission acknowledgement is invalid")
	}
	return &Client{connection: connection, privateKey: privateKey, actorKey: actorKey, roomID: roomID, sequence: 1}, nil
}

func (client *Client) ActorKey() protocolv2.ActorKey {
	return client.actorKey
}

func (client *Client) ReplaceAdmissions(ctx context.Context, generation uint64, admissions []protocolv2.AdmissionControl) error {
	return client.sendControl(ctx, protocolv2.KindAdmissionReplace, protocolv2.AdmissionReplaceControl{Generation: generation, Admissions: admissions})
}

func (client *Client) SendApplication(ctx context.Context, groupKey protocolv2.GroupKey, route protocolv2.Route, recipient protocolv2.ActorKey, messageType protocolv2.ApplicationType, payload any) error {
	encodedPayload, err := protocolv2.EncodeApplication(messageType, payload)
	if err != nil {
		return err
	}
	client.mutex.Lock()
	defer client.mutex.Unlock()
	header, err := client.nextHeader(route, protocolv2.KindApplication, recipient)
	if err != nil {
		return err
	}
	envelope, err := protocolv2.Seal(rand.Reader, groupKey, client.privateKey, header, encodedPayload)
	if err != nil {
		return err
	}
	return client.write(ctx, envelope)
}

func (client *Client) ReadApplication(ctx context.Context, groupKey protocolv2.GroupKey) (protocolv2.ActorKey, protocolv2.DecodedApplication, error) {
	messageType, encoded, err := client.connection.Read(ctx)
	if err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	if messageType != websocket.MessageBinary {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, fmt.Errorf("relay sent a non-binary frame")
	}
	envelope, err := protocolv2.UnmarshalEnvelope(encoded)
	if err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	if envelope.Header.RoomID != client.roomID || envelope.Header.Kind != protocolv2.KindApplication {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, fmt.Errorf("relay application frame has invalid routing context")
	}
	if err := protocolv2.VerifyEnvelopeSignature(envelope); err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	if err := client.acceptInbound(envelope.Header); err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	plaintext, err := protocolv2.Open(groupKey, ed25519.PublicKey(envelope.Header.Sender[:]), envelope)
	if err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	decoded, err := protocolv2.DecodeApplication(plaintext)
	if err != nil {
		return protocolv2.ActorKey{}, protocolv2.DecodedApplication{}, err
	}
	return envelope.Header.Sender, decoded, nil
}

func (client *Client) acceptInbound(header protocolv2.Header) error {
	client.receiveMutex.Lock()
	defer client.receiveMutex.Unlock()
	if client.validators == nil {
		client.validators = make(map[protocolv2.ActorKey]*protocolv2.SequenceValidator)
		client.seenMessages = make(map[protocolv2.MessageID]struct{})
	}
	validator := client.validators[header.Sender]
	if validator == nil {
		validator = &protocolv2.SequenceValidator{}
		client.validators[header.Sender] = validator
	}
	if err := validator.Accept(header); err != nil {
		return err
	}
	if _, seen := client.seenMessages[header.MessageID]; seen {
		return fmt.Errorf("relay application message ID was replayed")
	}
	client.seenMessages[header.MessageID] = struct{}{}
	client.seenOrder = append(client.seenOrder, header.MessageID)
	if len(client.seenOrder) > inboundReplayCacheSize {
		delete(client.seenMessages, client.seenOrder[0])
		client.seenOrder = client.seenOrder[1:]
	}
	return nil
}

func (client *Client) Close() error {
	return client.connection.Close(websocket.StatusNormalClosure, "client closed")
}

func (client *Client) sendControl(ctx context.Context, kind protocolv2.Kind, payload any) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	header, err := client.nextHeader(protocolv2.RouteControl, kind, protocolv2.ActorKey{})
	if err != nil {
		return err
	}
	envelope, err := protocolv2.SignControl(rand.Reader, client.privateKey, header, payload)
	if err != nil {
		return err
	}
	return client.write(ctx, envelope)
}

func (client *Client) nextHeader(route protocolv2.Route, kind protocolv2.Kind, recipient protocolv2.ActorKey) (protocolv2.Header, error) {
	client.sequence++
	header := protocolv2.Header{Version: protocolv2.Version, Route: route, Kind: kind, RoomID: client.roomID, Sender: client.actorKey, Recipient: recipient, Sequence: client.sequence}
	if _, err := rand.Read(header.MessageID[:]); err != nil {
		return protocolv2.Header{}, fmt.Errorf("generate relay message ID: %w", err)
	}
	return header, nil
}

func (client *Client) write(ctx context.Context, envelope protocolv2.Envelope) error {
	encoded, err := protocolv2.MarshalEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := client.connection.Write(ctx, websocket.MessageBinary, encoded); err != nil {
		return fmt.Errorf("write relay frame: %w", err)
	}
	return nil
}

func relayWebSocketURL(endpoint string) (string, string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", "", fmt.Errorf("relay endpoint is invalid")
	}
	loopback := parsed.Hostname() == "localhost"
	if address := net.ParseIP(parsed.Hostname()); address != nil && address.IsLoopback() {
		loopback = true
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		if !loopback {
			return "", "", fmt.Errorf("relay endpoint must use HTTPS outside loopback")
		}
		parsed.Scheme = "ws"
	default:
		return "", "", fmt.Errorf("relay endpoint scheme is unsupported")
	}
	origin := endpoint
	parsed.Path = "/v2/collaboration"
	return parsed.String(), origin, nil
}

func ValidateEndpoint(endpoint string) error {
	_, _, err := relayWebSocketURL(endpoint)
	return err
}
