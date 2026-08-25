package model

import "time"

type OperationKind string

const (
	OperationKindTileChange OperationKind = "tile_change"
	OperationKindInverse    OperationKind = "inverse"
)

type TileChange struct {
	Coord  Coord     `json:"coord"`
	Before TileState `json:"before"`
	After  TileState `json:"after"`
}

type Operation struct {
	ProtocolVersion uint16        `json:"protocol_version"`
	DocumentID      DocumentID    `json:"document_id"`
	ActorID         ActorID       `json:"actor_id"`
	OperationID     OperationID   `json:"operation_id"`
	BaseRevision    Revision      `json:"base_revision"`
	EnvironmentHash string        `json:"environment_hash"`
	BaseMapHash     string        `json:"base_map_hash"`
	Kind            OperationKind `json:"kind"`
	Changes         []TileChange  `json:"changes"`
	InverseOf       *OperationID  `json:"inverse_of,omitempty"`
}

type AcceptedOperation struct {
	Operation
	Revision   Revision  `json:"revision"`
	AcceptedAt time.Time `json:"accepted_at"`
}
