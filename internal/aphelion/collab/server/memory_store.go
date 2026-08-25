package server

import collabstore "sdmm/internal/aphelion/collab/store"

var (
	ErrSessionExists  = collabstore.ErrSessionExists
	ErrSessionMissing = collabstore.ErrSessionMissing
	ErrStoreClosed    = collabstore.ErrStoreClosed
)

type SessionStore = collabstore.SessionStore
type MemoryStore = collabstore.MemoryStore

func NewMemoryStore() *MemoryStore {
	return collabstore.NewMemoryStore()
}
