package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

const collaborationSubprotocol = "apheliondmm.collaboration.v1"

var ErrTransportNotConnected = errors.New("collaboration transport is not connected")

type TransportConfig struct {
	DurableQueueSize int
	DialTimeout      time.Duration
	WriteTimeout     time.Duration
}

type WebSocketTransport struct {
	config TransportConfig

	mutex       sync.RWMutex
	connection  *websocket.Conn
	cancel      context.CancelFunc
	durable     chan protocol.ClientEnvelope
	presence    chan protocol.ClientEnvelope
	done        chan struct{}
	terminalErr error
	presenceMu  sync.Mutex
	errorOnce   sync.Once
}

func NewWebSocketTransport(config TransportConfig) *WebSocketTransport {
	if config.DurableQueueSize <= 0 {
		config.DurableQueueSize = 128
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = 5 * time.Second
	}
	return &WebSocketTransport{config: config}
}

func (transport *WebSocketTransport) Connect(ctx context.Context, request protocol.JoinRequest, receive func(protocol.ServerEnvelope)) error {
	if receive == nil {
		return fmt.Errorf("receive callback is nil")
	}
	websocketURL, err := collaborationURL(request.BaseURL)
	if err != nil {
		return err
	}
	if request.Token == "" || request.SessionID == "" || request.Origin == "" {
		return fmt.Errorf("join token, session id, and origin are required")
	}

	transport.mutex.Lock()
	if transport.connection != nil {
		transport.mutex.Unlock()
		return fmt.Errorf("transport is already connected")
	}
	transport.mutex.Unlock()
	dialContext, cancelDial := context.WithTimeout(ctx, transport.config.DialTimeout)
	connection, response, err := websocket.Dial(dialContext, websocketURL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"Authorization": []string{"Bearer " + request.Token}, "Origin": []string{request.Origin}},
		Subprotocols: []string{collaborationSubprotocol},
	})
	cancelDial()
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			switch response.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return fmt.Errorf("%w: HTTP %d", ErrAuthenticationDenied, response.StatusCode)
			case http.StatusUpgradeRequired, http.StatusBadRequest:
				return fmt.Errorf("%w: HTTP %d", ErrIncompatibleProtocol, response.StatusCode)
			}
		}
		return fmt.Errorf("connect collaboration WebSocket: %w", err)
	}
	connection.SetReadLimit(protocol.MaxMessageBytes)
	connectionContext, cancelConnection := context.WithCancel(context.WithoutCancel(ctx))
	transport.mutex.Lock()
	transport.connection = connection
	transport.cancel = cancelConnection
	transport.durable = make(chan protocol.ClientEnvelope, transport.config.DurableQueueSize)
	transport.presence = make(chan protocol.ClientEnvelope, 1)
	transport.done = make(chan struct{})
	transport.mutex.Unlock()

	joinPayload, err := json.Marshal(protocol.JoinPayload{JoinToken: request.Token, AcknowledgedRevision: request.AcknowledgedRevision})
	if err != nil {
		cancelConnection()
		_ = connection.CloseNow()
		return err
	}
	join := protocol.ClientEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "join", SessionID: request.SessionID, Type: protocol.ClientJoin, Payload: joinPayload}
	if err := transport.write(connectionContext, connection, join); err != nil {
		cancelConnection()
		_ = connection.CloseNow()
		return fmt.Errorf("send collaboration join: %w", err)
	}

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		transport.readLoop(connectionContext, connection, receive)
	}()
	go func() {
		defer workers.Done()
		transport.writeLoop(connectionContext, connection)
	}()
	go func() {
		workers.Wait()
		_ = connection.CloseNow()
		transport.mutex.RLock()
		done := transport.done
		transport.mutex.RUnlock()
		close(done)
	}()
	return nil
}

func (transport *WebSocketTransport) Send(ctx context.Context, message protocol.ClientEnvelope) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := protocol.DecodeClient(data); err != nil {
		return err
	}
	transport.mutex.RLock()
	durable, presence, done := transport.durable, transport.presence, transport.done
	transport.mutex.RUnlock()
	if durable == nil || presence == nil || done == nil {
		return ErrTransportNotConnected
	}
	if message.Type == protocol.ClientPresenceUpdate {
		transport.presenceMu.Lock()
		defer transport.presenceMu.Unlock()
		select {
		case presence <- message:
			return nil
		default:
			select {
			case <-presence:
			default:
			}
			select {
			case presence <- message:
				return nil
			case <-done:
				return transport.result()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	select {
	case durable <- message:
		return nil
	case <-done:
		return transport.result()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (transport *WebSocketTransport) Close(status websocket.StatusCode, reason string) error {
	transport.mutex.RLock()
	connection, cancel := transport.connection, transport.cancel
	transport.mutex.RUnlock()
	if connection == nil || cancel == nil {
		return ErrTransportNotConnected
	}
	err := connection.Close(status, reason)
	cancel()
	return err
}

func (transport *WebSocketTransport) Wait(ctx context.Context) error {
	transport.mutex.RLock()
	done := transport.done
	transport.mutex.RUnlock()
	if done == nil {
		return ErrTransportNotConnected
	}
	select {
	case <-done:
		return transport.result()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (transport *WebSocketTransport) readLoop(ctx context.Context, connection *websocket.Conn, receive func(protocol.ServerEnvelope)) {
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			transport.finish(err)
			return
		}
		decoded, err := protocol.DecodeServer(data)
		if err != nil {
			transport.finish(fmt.Errorf("decode server envelope: %w", err))
			return
		}
		receive(decoded.Envelope)
	}
}

func (transport *WebSocketTransport) writeLoop(ctx context.Context, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			transport.finish(ctx.Err())
			return
		case message := <-transport.durable:
			if err := transport.write(ctx, connection, message); err != nil {
				transport.finish(err)
				return
			}
		case message := <-transport.presence:
			if err := transport.write(ctx, connection, message); err != nil {
				transport.finish(err)
				return
			}
		}
	}
}

func (transport *WebSocketTransport) write(ctx context.Context, connection *websocket.Conn, message protocol.ClientEnvelope) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	writeContext, cancel := context.WithTimeout(ctx, transport.config.WriteTimeout)
	defer cancel()
	return connection.Write(writeContext, websocket.MessageText, data)
}

func (transport *WebSocketTransport) finish(err error) {
	transport.errorOnce.Do(func() {
		transport.mutex.Lock()
		transport.terminalErr = err
		cancel := transport.cancel
		transport.mutex.Unlock()
		if cancel != nil {
			cancel()
		}
	})
}

func (transport *WebSocketTransport) result() error {
	transport.mutex.RLock()
	defer transport.mutex.RUnlock()
	return transport.terminalErr
}

func collaborationURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse collaboration base URL: %w", err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("collaboration base URL cannot contain credentials, query, or fragment")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		ip := net.ParseIP(parsed.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("cleartext collaboration base URL is restricted to loopback IP addresses")
		}
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("collaboration base URL scheme must be http or https")
	}
	parsed.Path = "/v1/collaboration"
	return parsed.String(), nil
}
