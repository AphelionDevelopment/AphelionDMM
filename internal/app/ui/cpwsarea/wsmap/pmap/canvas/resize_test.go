package canvas

import (
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/SpaiR/imgui-go"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"sdmm/internal/app/render/brush"
	"sdmm/internal/util"
)

var resizeWindow *glfw.Window

// Keep one native context for serial tests and benchmark calibration passes.
// Brush caches are process scoped, so recreating the context between benchmark
// invocations would give them resource names from a destroyed context.
func TestMain(m *testing.M) {
	if os.Getenv("APHELIONDMM_GL_TEST") != "1" {
		os.Exit(m.Run())
	}
	runtime.LockOSThread()
	if err := glfw.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	win, err := glfw.CreateWindow(64, 64, "Canvas resize verification", nil, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		glfw.Terminate()
		os.Exit(1)
	}
	win.MakeContextCurrent()
	if err = gl.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		win.Destroy()
		glfw.Terminate()
		os.Exit(1)
	}
	resizeWindow = win
	glfw.DetachCurrentContext()
	code := m.Run()
	win.MakeContextCurrent()
	brush.Dispose()
	win.Destroy()
	glfw.Terminate()
	os.Exit(code)
}

func resizeContext(tb testing.TB) {
	tb.Helper()
	if resizeWindow == nil {
		tb.Skip("set APHELIONDMM_GL_TEST=1 for real framebuffer resize verification")
	}
	runtime.LockOSThread()
	tb.Cleanup(runtime.UnlockOSThread)
	resizeWindow.MakeContextCurrent()
	tb.Cleanup(glfw.DetachCurrentContext)
	tb.Logf("GL renderer=%s version=%s", gl.GoStr(gl.GetString(gl.RENDERER)), gl.GoStr(gl.GetString(gl.VERSION)))
}

func resizeCanvas(tb testing.TB) *Canvas {
	tb.Helper()
	c := New()
	c.ClearColor = Color{R: 1, B: 1, A: 1}
	tb.Cleanup(func() { gl.DeleteTextures(1, &c.texture); gl.DeleteFramebuffers(1, &c.frameBuffer) })
	return c
}

func checkResizePixels(t *testing.T, c *Canvas, size imgui.Vec2, marker bool) {
	t.Helper()
	if code := gl.GetError(); code != gl.NO_ERROR {
		t.Fatalf("GL error after resize: 0x%x", code)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, c.frameBuffer)
	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		t.Fatalf("incomplete framebuffer: 0x%x", status)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	pixels := c.ReadPixels()
	for y := 0; y < int(size.Y); y++ {
		for x := 0; x < int(size.X); x++ {
			want := [4]byte{255, 0, 255, 255}
			if marker && x >= 1 && x < 4 && y >= 1 && y < 4 {
				want = [4]byte{0, 255, 0, 255}
			}
			i := 4 * (y*int(size.X) + x)
			if got := [4]byte{pixels[i], pixels[i+1], pixels[i+2], pixels[i+3]}; got != want {
				t.Fatalf("pixel (%d,%d) size=%v got=%v want=%v", x, y, size, got, want)
			}
		}
	}
	t.Logf("pixels size=%dx%d marker=%v sha256=%x", int(size.X), int(size.Y), marker, sha256.Sum256(pixels))
}

func TestCanvasResizePixelsAndAllocation(t *testing.T) {
	resizeContext(t)
	c := resizeCanvas(t)
	for _, size := range []imgui.Vec2{{X: 1, Y: 1}, {X: 7, Y: 5}, {X: 640, Y: 480}, {X: 641, Y: 481}, {X: 320, Y: 240}, {X: 640, Y: 480}} {
		c.Process(size)
		checkResizePixels(t, c, size, false)
		if size.X >= 4 && size.Y >= 4 {
			brush.RectFilled(1, 1, 4, 4, util.MakeColor(0, 1, 0, 1))
			c.Process(size)
			checkResizePixels(t, c, size, true)
			c.Process(size)
			checkResizePixels(t, c, size, false)
		}
	}
	const iterations = 10
	c.Process(imgui.Vec2{X: 641, Y: 481})
	gl.Finish()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < iterations; i++ {
		c.Process(imgui.Vec2{X: float32(640 + i%2), Y: float32(480 + i%2)})
		gl.Finish()
	}
	runtime.ReadMemStats(&after)
	bytes := (after.TotalAlloc - before.TotalAlloc) / iterations
	t.Logf("resize_cpu_bytes_per_iteration=%d", bytes)
	if bytes > 64*1024 {
		t.Fatalf("canvas resize allocates pixel-sized CPU storage: %d bytes/iteration", bytes)
	}
}

// One process selects one fixed fixture size. Alternate dimensions force every
// timed Canvas.Process call to resize; Finish includes completion of GPU work.
func BenchmarkCanvasResize(b *testing.B) {
	resizeContext(b)
	c := resizeCanvas(b)
	width, height := 640, 480
	if os.Getenv("APHELION_CANVAS_BENCH_SIZE") == "1920x1080" {
		width, height = 1920, 1080
	}
	c.Process(imgui.Vec2{X: float32(width + 1), Y: float32(height + 1)})
	gl.Finish()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Process(imgui.Vec2{X: float32(width + i%2), Y: float32(height + i%2)})
		gl.Finish()
	}
	b.StopTimer()
	if code := gl.GetError(); code != gl.NO_ERROR {
		b.Fatalf("GL error: 0x%x", code)
	}
	// Validate the complete final target outside the timed/allocation interval,
	// including the larger benchmark fixture absent from the small pixel sweep.
	pixels := c.ReadPixels()
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] != 255 || pixels[i+1] != 0 || pixels[i+2] != 255 || pixels[i+3] != 255 {
			b.Fatalf("uncleared final pixel %d", i/4)
		}
	}
	b.Logf("result size=%dx%d sha256=%x", int(c.width), int(c.height), sha256.Sum256(pixels))
}
