// APHELION EDIT ADDITION START - SELECTION LIFECYCLE
package tools

import (
	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/aphelion/hotkeys"
)

func cancelGrabOnEscape() {
	grab, ok := Selected().(*ToolGrab)
	if !ok || grab.Stale() || imgui.CurrentIO().WantTextInput() {
		return
	}
	if hotkeys.Pressed([][2]glfw.Key{{glfw.KeyEscape, 0}}, func(key glfw.Key) bool { return imgui.IsKeyDown(int(key)) }, func(key glfw.Key) bool { return imgui.IsKeyPressedV(int(key), false) }) {
		grab.OnDeselect()
	}
}

// APHELION EDIT ADDITION END
