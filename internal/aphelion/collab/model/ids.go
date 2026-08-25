package model

import (
	"fmt"

	"github.com/google/uuid"
)

type DocumentID string

type ActorID string

type OperationID string

type StableID string

func NewDocumentID() (DocumentID, error) {
	value, err := newUUIDv7()
	return DocumentID(value), err
}

func NewActorID() (ActorID, error) {
	value, err := newUUIDv7()
	return ActorID(value), err
}

func NewOperationID() (OperationID, error) {
	value, err := newUUIDv7()
	return OperationID(value), err
}

func NewStableID() (StableID, error) {
	value, err := newUUIDv7()
	return StableID(value), err
}

func (id DocumentID) Validate() error {
	return validateUUIDv7("document id", string(id))
}

func (id ActorID) Validate() error {
	return validateUUIDv7("actor id", string(id))
}

func (id OperationID) Validate() error {
	return validateUUIDv7("operation id", string(id))
}

func (id StableID) Validate() error {
	return validateUUIDv7("stable id", string(id))
}

func newUUIDv7() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return value.String(), nil
}

func validateUUIDv7(name string, value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return fmt.Errorf("%s: parse UUID: %w", name, err)
	}
	if parsed.Version() != 7 {
		return fmt.Errorf("%s: UUID version is %d, want 7", name, parsed.Version())
	}
	if parsed.Variant() != uuid.RFC4122 {
		return fmt.Errorf("%s: UUID variant is %s, want RFC 4122", name, parsed.Variant())
	}
	return nil
}
