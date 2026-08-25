package client

import (
	"context"

	"github.com/coder/websocket"

	"sdmm/internal/aphelion/collab/protocol"
)

type Transport interface {
	Connect(context.Context, protocol.JoinRequest, func(protocol.ServerEnvelope)) error
	Send(context.Context, protocol.ClientEnvelope) error
	Close(websocket.StatusCode, string) error
}
