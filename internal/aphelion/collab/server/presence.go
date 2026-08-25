package server

import (
	"fmt"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

type PresenceUpdate struct {
	Sequence uint64
	Cursor   *model.Coord
	Status   string
}

type Presence struct {
	ActorID     model.ActorID
	DisplayName string
	Sequence    uint64
	Cursor      *model.Coord
	Status      string
	UpdatedAt   time.Time
}

type PresenceManager struct {
	mutex       sync.Mutex
	timeout     time.Duration
	current     map[model.ActorID]Presence
	subscribers map[uint64]chan Presence
	nextID      uint64
}

func NewPresenceManager(timeout time.Duration) *PresenceManager {
	if timeout <= 0 {
		timeout = time.Minute
	}
	return &PresenceManager{
		timeout:     timeout,
		current:     make(map[model.ActorID]Presence),
		subscribers: make(map[uint64]chan Presence),
	}
}

func (manager *PresenceManager) Update(principal Principal, update PresenceUpdate) error {
	return manager.updateAt(principal, update, time.Now().UTC())
}

func (manager *PresenceManager) updateAt(principal Principal, update PresenceUpdate, updatedAt time.Time) error {
	if update.Sequence == 0 {
		return fmt.Errorf("presence sequence must be positive")
	}
	if update.Status == "" || len(update.Status) > 128 {
		return fmt.Errorf("presence status length is %d, want 1..128", len(update.Status))
	}
	if update.Cursor != nil && (update.Cursor.X < 1 || update.Cursor.Y < 1 || update.Cursor.Z < 1) {
		return fmt.Errorf("presence cursor coordinates must be positive")
	}

	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if current, exists := manager.current[principal.ActorID()]; exists && update.Sequence <= current.Sequence {
		return fmt.Errorf("presence sequence is %d, current is %d", update.Sequence, current.Sequence)
	}
	value := Presence{
		ActorID:     principal.ActorID(),
		DisplayName: principal.DisplayName(),
		Sequence:    update.Sequence,
		Cursor:      cloneCoord(update.Cursor),
		Status:      update.Status,
		UpdatedAt:   updatedAt,
	}
	manager.current[value.ActorID] = value
	manager.publish(value)
	return nil
}

func (manager *PresenceManager) Remove(actorID model.ActorID) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	delete(manager.current, actorID)
}

func (manager *PresenceManager) Expire(now time.Time) int {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	removed := 0
	for actorID, presence := range manager.current {
		if now.Sub(presence.UpdatedAt) >= manager.timeout {
			delete(manager.current, actorID)
			removed++
		}
	}
	return removed
}

func (manager *PresenceManager) Subscribe(buffer int) ([]Presence, <-chan Presence, func()) {
	if buffer < 1 {
		buffer = 1
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.nextID++
	id := manager.nextID
	updates := make(chan Presence, buffer)
	manager.subscribers[id] = updates
	snapshot := make([]Presence, 0, len(manager.current))
	for _, presence := range manager.current {
		presence.Cursor = cloneCoord(presence.Cursor)
		snapshot = append(snapshot, presence)
	}
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			manager.mutex.Lock()
			defer manager.mutex.Unlock()
			if subscriber, exists := manager.subscribers[id]; exists {
				delete(manager.subscribers, id)
				close(subscriber)
			}
		})
	}
	return snapshot, updates, cancel
}

func (manager *PresenceManager) publish(value Presence) {
	for _, subscriber := range manager.subscribers {
		copy := value
		copy.Cursor = cloneCoord(value.Cursor)
		select {
		case subscriber <- copy:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- copy:
			default:
			}
		}
	}
}

func cloneCoord(coord *model.Coord) *model.Coord {
	if coord == nil {
		return nil
	}
	copy := *coord
	return &copy
}
