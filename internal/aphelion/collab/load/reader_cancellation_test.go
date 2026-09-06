package load

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

type cancelledLoadRead struct {
	payload []byte
	read    bool
}

func (reader *cancelledLoadRead) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	// A successful buffered read may race with cancellation. The reader must
	// still be able to stop while its consumer is gone or its event queue is full.
	<-ctx.Done()
	if reader.read {
		return websocket.MessageText, nil, ctx.Err()
	}
	reader.read = true
	return websocket.MessageText, reader.payload, nil
}

func TestLoadReaderCancellationDoesNotWaitForConsumer(t *testing.T) {
	scenario, err := Generate(Config{Seed: 17, Clients: 1, Operations: 1, MaxX: 1, MaxY: 1})
	if err != nil {
		t.Fatal(err)
	}
	document, err := engine.NewDocument(scenario.Initial)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := document.Apply(scenario.Operations[0], time.Unix(0, 1))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := document.Snapshot().Hash()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(protocol.OperationAcceptedPayload{Operation: accepted, MapHash: hash})
	envelope, _ := json.Marshal(protocol.ServerEnvelope{ProtocolVersion: model.ProtocolVersion, MessageID: "accepted", SessionID: "load-test", Type: protocol.ServerOperationAccepted, Payload: payload})
	for _, data := range [][]byte{envelope, []byte("invalid")} {
		ctx, cancel := context.WithCancel(context.Background())
		events, failures := make(chan acceptedEvent), make(chan error)
		done := make(chan struct{})
		go func() { readLoadEvents(ctx, 0, &cancelledLoadRead{payload: data}, events, failures); close(done) }()
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			// Release the old implementation before failing rather than leak it.
			go func() {
				select {
				case <-events:
				case <-failures:
				}
			}()
			t.Fatal("cancelled load reader waits on an unconsumed event/error channel")
		}
	}
}
