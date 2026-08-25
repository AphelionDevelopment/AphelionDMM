package pmap

import (
	"testing"

	collabui "sdmm/internal/aphelion/collab/ui"

	"github.com/SpaiR/imgui-go"
)

func TestPresenceScreenBoundsApplyCameraAndInvertYAxis(t *testing.T) {
	t.Parallel()

	overlay := collabui.PresenceOverlay{PixelX: 32, PixelY: 64}
	min, max := presenceScreenBounds(overlay, 32, 2, 10, -5, imgui.Vec2{X: 100, Y: 200}, imgui.Vec2{X: 500, Y: 600})

	if min != (imgui.Vec2{X: 184, Y: 418}) || max != (imgui.Vec2{X: 248, Y: 482}) {
		t.Fatalf("screen bounds = %v..%v, want (184,418)..(248,482)", min, max)
	}
}

func TestPresenceSelectionScreenBoundsApplyCameraAndInvertYAxis(t *testing.T) {
	t.Parallel()

	selection := collabui.PresenceSelectionOverlay{PixelX1: 32, PixelY1: 64, PixelX2: 96, PixelY2: 128}
	min, max := presenceSelectionScreenBounds(selection, 2, 10, -5, imgui.Vec2{X: 100, Y: 200}, imgui.Vec2{X: 500, Y: 600})

	if min != (imgui.Vec2{X: 184, Y: 354}) || max != (imgui.Vec2{X: 312, Y: 482}) {
		t.Fatalf("selection screen bounds = %v..%v, want (184,354)..(312,482)", min, max)
	}
}
