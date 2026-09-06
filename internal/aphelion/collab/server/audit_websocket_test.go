package server

import (
	"context"
	"sdmm/internal/aphelion/collab/protocol"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuditRejectionFollowsItsAuthorityOnWebSocket(t *testing.T) {
	var armed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	service, created, host := startHTTPTestSessionWithConfig(t, ServiceConfig{
		AllowedOrigins: []string{"http://127.0.0.1"},
		Now: func() time.Time {
			if armed.CompareAndSwap(true, false) {
				close(entered)
				<-release
			}
			return time.Now()
		},
	})
	owner := service.sessions[created.SessionID].owner
	snapshot, err := owner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	connection := connectTestClient(t, host.URL, created.SessionID, created.OwnerToken, 0)
	defer func() { _ = connection.CloseNow() }()
	armed.Store(true)
	submitTestOperation(t, connection, created.SessionID, "delayed", testOperation(t, snapshot, 1))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("submission did not enter limiter")
	}
	// While this socket's sole writer handles the submitted message, another
	// request commits a competing edit and enqueues its durable notification.
	if _, err := owner.Submit(context.Background(), testOperation(t, snapshot, 1)); err != nil {
		t.Fatal(err)
	}
	unblock.Do(func() { close(release) })
	for {
		message := readServerEnvelope(t, connection)
		if message.Envelope.Type == protocol.ServerPresenceSnapshot {
			continue
		}
		if message.Envelope.Type != protocol.ServerOperationAccepted {
			t.Fatalf("first durable result = %s; rejection preceded its authority", message.Envelope.Type)
		}
		break
	}
	rejected := readServerEnvelopeType(t, connection, protocol.ServerOperationRejected).Payload.(*protocol.OperationRejectedPayload)
	if rejected.Revision != 1 {
		t.Fatalf("rejection revision = %d", rejected.Revision)
	}
}
