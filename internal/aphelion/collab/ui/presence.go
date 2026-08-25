package ui

import (
	"hash/fnv"
	"sort"
	"time"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

const presenceStyleSlotCount = 8

type ObservedPresence struct {
	Presence   protocol.ParticipantPresence
	ObservedAt time.Time
}

type PresenceOverlay struct {
	ActorID   model.ActorID
	Label     string
	PixelX    int
	PixelY    int
	StyleSlot uint32
}

func BuildPresenceOverlays(entries []ObservedPresence, activeLevel, iconSize, participantCap int, timeout time.Duration, now time.Time) []PresenceOverlay {
	if activeLevel < 1 || iconSize < 1 || participantCap < 1 || timeout <= 0 {
		return nil
	}
	visible := make([]ObservedPresence, 0, len(entries))
	for _, entry := range entries {
		cursor := entry.Presence.Cursor
		if cursor == nil || cursor.Z != activeLevel || cursor.X < 1 || cursor.Y < 1 || now.Sub(entry.ObservedAt) > timeout {
			continue
		}
		visible = append(visible, entry)
	}
	sort.Slice(visible, func(left, right int) bool {
		return visible[left].Presence.ActorID < visible[right].Presence.ActorID
	})
	if len(visible) > participantCap {
		visible = visible[:participantCap]
	}
	overlays := make([]PresenceOverlay, len(visible))
	for index, entry := range visible {
		label := entry.Presence.DisplayName
		if label == "" {
			label = string(entry.Presence.ActorID)
		}
		overlays[index] = PresenceOverlay{
			ActorID:   entry.Presence.ActorID,
			Label:     label,
			PixelX:    (entry.Presence.Cursor.X - 1) * iconSize,
			PixelY:    (entry.Presence.Cursor.Y - 1) * iconSize,
			StyleSlot: presenceStyleSlot(entry.Presence.ActorID),
		}
	}
	return overlays
}

func presenceStyleSlot(actorID model.ActorID) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(actorID))
	return hash.Sum32() % presenceStyleSlotCount
}
