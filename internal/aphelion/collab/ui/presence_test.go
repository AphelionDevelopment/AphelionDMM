package ui

import (
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestBuildPresenceOverlaysFiltersBoundsAndConvertsCoordinates(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	entries := []ObservedPresence{
		{Presence: protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bc"), DisplayName: "Second", Cursor: coord(3, 2, 1)}, ObservedAt: now},
		{Presence: protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bb"), Cursor: coord(2, 4, 1), Selection: &protocol.PresenceSelection{Min: model.Coord{X: 1, Y: 2, Z: 1}, Max: model.Coord{X: 2, Y: 3, Z: 1}}}, ObservedAt: now},
		{Presence: protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bd"), DisplayName: "Capped", Cursor: coord(1, 1, 1)}, ObservedAt: now},
		{Presence: protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789be"), Cursor: coord(1, 1, 2)}, ObservedAt: now},
		{Presence: protocol.ParticipantPresence{ActorID: model.ActorID("01890f3e-7b5c-7abc-8def-0123456789bf"), Cursor: coord(1, 1, 1)}, ObservedAt: now.Add(-time.Minute)},
	}
	overlays := BuildPresenceOverlays(entries, 1, 32, 2, 10*time.Second, now)
	if len(overlays) != 2 {
		t.Fatalf("overlays = %d, want 2", len(overlays))
	}
	if overlays[0].PixelX != 32 || overlays[0].PixelY != 96 || overlays[0].Label == "" {
		t.Fatalf("first overlay = %#v", overlays[0])
	}
	if overlays[0].Selection == nil || overlays[0].Selection.PixelX1 != 0 || overlays[0].Selection.PixelY1 != 32 || overlays[0].Selection.PixelX2 != 64 || overlays[0].Selection.PixelY2 != 96 {
		t.Fatalf("first selection overlay = %#v", overlays[0].Selection)
	}
	if overlays[1].PixelX != 64 || overlays[1].PixelY != 32 || overlays[1].Label != "Second" {
		t.Fatalf("second overlay = %#v", overlays[1])
	}
}

func coord(x, y, z int) *model.Coord {
	value := model.Coord{X: x, Y: y, Z: z}
	return &value
}
