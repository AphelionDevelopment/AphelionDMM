package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type DocumentID string

type ActorID string

type OperationID string

type StableID string

type CheckpointID string

var uuidV7State struct {
	sync.Mutex
	initialized bool
	last        [16]byte
}

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

func NewCheckpointID() (CheckpointID, error) {
	value, err := newUUIDv7()
	return CheckpointID(value), err
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

func (id CheckpointID) Validate() error {
	return validateUUIDv7("checkpoint id", string(id))
}

func newUUIDv7() (string, error) {
	uuidV7State.Lock()
	defer uuidV7State.Unlock()

	var value [16]byte
	timestamp := uint64(time.Now().UnixMilli())
	if uuidV7State.initialized && timestamp <= uuidV7Timestamp(uuidV7State.last) {
		value = uuidV7State.last
		if !incrementUUIDv7Random(&value) {
			timestamp = uuidV7Timestamp(uuidV7State.last) + 1
			setUUIDv7Timestamp(&value, timestamp)
			for index := 6; index < len(value); index++ {
				value[index] = 0
			}
			value[6] = 0x70
			value[8] = 0x80
		}
	} else {
		if _, err := rand.Read(value[6:]); err != nil {
			return "", fmt.Errorf("generate UUIDv7 randomness: %w", err)
		}
		setUUIDv7Timestamp(&value, timestamp)
		value[6] = value[6]&0x0f | 0x70
		value[8] = value[8]&0x3f | 0x80
	}
	uuidV7State.last = value
	uuidV7State.initialized = true

	var compact [32]byte
	hex.Encode(compact[:], value[:])
	var encoded [36]byte
	copy(encoded[0:8], compact[0:8])
	encoded[8] = '-'
	copy(encoded[9:13], compact[8:12])
	encoded[13] = '-'
	copy(encoded[14:18], compact[12:16])
	encoded[18] = '-'
	copy(encoded[19:23], compact[16:20])
	encoded[23] = '-'
	copy(encoded[24:36], compact[20:32])
	return string(encoded[:]), nil
}

func uuidV7Timestamp(value [16]byte) uint64 {
	return uint64(value[0])<<40 | uint64(value[1])<<32 | uint64(value[2])<<24 |
		uint64(value[3])<<16 | uint64(value[4])<<8 | uint64(value[5])
}

func setUUIDv7Timestamp(value *[16]byte, timestamp uint64) {
	value[0] = byte(timestamp >> 40)
	value[1] = byte(timestamp >> 32)
	value[2] = byte(timestamp >> 24)
	value[3] = byte(timestamp >> 16)
	value[4] = byte(timestamp >> 8)
	value[5] = byte(timestamp)
}

func incrementUUIDv7Random(value *[16]byte) bool {
	for index := 15; index >= 9; index-- {
		value[index]++
		if value[index] != 0 {
			return true
		}
	}
	variantRandom := value[8] & 0x3f
	if variantRandom != 0x3f {
		value[8] = 0x80 | (variantRandom + 1)
		return true
	}
	value[8] = 0x80
	value[7]++
	if value[7] != 0 {
		return true
	}
	versionRandom := value[6] & 0x0f
	if versionRandom != 0x0f {
		value[6] = 0x70 | (versionRandom + 1)
		return true
	}
	return false
}

func validateUUIDv7(name string, value string) error {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return fmt.Errorf("%s: UUID must use canonical 8-4-4-4-12 text", name)
	}
	var compact [32]byte
	copy(compact[0:8], value[0:8])
	copy(compact[8:12], value[9:13])
	copy(compact[12:16], value[14:18])
	copy(compact[16:20], value[19:23])
	copy(compact[20:32], value[24:36])
	var decoded [16]byte
	if _, err := hex.Decode(decoded[:], compact[:]); err != nil {
		return fmt.Errorf("%s: parse UUID: %w", name, err)
	}
	if decoded[6]>>4 != 7 {
		return fmt.Errorf("%s: UUID version is %d, want 7", name, decoded[6]>>4)
	}
	if decoded[8]&0xc0 != 0x80 {
		return fmt.Errorf("%s: UUID variant is not RFC 4122", name)
	}
	return nil
}
