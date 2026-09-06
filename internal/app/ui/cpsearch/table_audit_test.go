package cpsearch

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"testing"
	"unsafe"

	"github.com/SpaiR/imgui-go"
)

func searchUI(tb testing.TB) {
	tb.Helper()
	runtime.LockOSThread()
	tb.Cleanup(runtime.UnlockOSThread)
	ctx := imgui.CreateContext(nil)
	tb.Cleanup(ctx.Destroy)
	io := imgui.CurrentIO()
	io.SetIniFilename("")
	io.SetDisplaySize(imgui.Vec2{X: 640, Y: 480})
	io.Fonts().TextureDataRGBA32()
}

func searchTableFrame(s *Search) (scroll, maximum float32) {
	imgui.NewFrame()
	imgui.SetNextWindowPosV(imgui.Vec2{}, imgui.ConditionAlways, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 600, Y: 400}, imgui.ConditionAlways)
	imgui.BeginV("Search table audit", nil, imgui.WindowFlagsNoSavedSettings)
	if imgui.BeginChild("results") {
		s.showResults()
		scroll, maximum = imgui.ScrollY(), imgui.ScrollMaxY()
	}
	imgui.EndChild()
	imgui.End()
	imgui.Render()
	return
}

func searchDrawHash() string {
	hash := sha256.New()
	for _, list := range imgui.RenderedDrawData().CommandLists() {
		vertices, vertexBytes := list.VertexBuffer()
		indices, indexBytes := list.IndexBuffer()
		_, _ = hash.Write(unsafe.Slice((*byte)(vertices), vertexBytes))
		_, _ = hash.Write(unsafe.Slice((*byte)(indices), indexBytes))
		for _, command := range list.Commands() {
			_, _ = fmt.Fprintf(hash, "%v:%d", command.ClipRect(), command.ElementCount())
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func TestSearchTableWorkScalesWithViewport(t *testing.T) {
	searchUI(t)
	var baseline float64
	for _, count := range []int{100, 5000} {
		s := searchFixture(t, count, 1)
		s.SearchByPath("/obj/search")
		for range 3 {
			searchTableFrame(s)
		}
		allocs := testing.AllocsPerRun(3, func() { searchTableFrame(s) })
		t.Logf("results=%d frame allocations=%v", count, allocs)
		if count == 100 {
			baseline = allocs
		} else if allocs > baseline+1000 {
			t.Errorf("off-screen result count dominates frame work: small=%v large=%v", baseline, allocs)
		}
	}
}

func TestSearchTableFocusScrollSurvivesClipping(t *testing.T) {
	searchUI(t)
	s := searchFixture(t, 5000, 1)
	s.SearchByPath("/obj/search")
	for range 3 {
		searchTableFrame(s)
	}
	s.focusedResultIdx = 4999
	var scroll, maximum float32
	for range 3 {
		scroll, maximum = searchTableFrame(s)
	}
	if maximum < 10000 || scroll < maximum-100 {
		t.Fatalf("last result not exposed: scroll=%v max=%v", scroll, maximum)
	}
	s.focusedResultIdx = 0
	for range 3 {
		scroll, _ = searchTableFrame(s)
	}
	if scroll > 30 {
		t.Fatalf("first result not exposed: %v", scroll)
	}
}

func BenchmarkSearchTable(b *testing.B) {
	searchUI(b)
	for _, count := range []int{100, 5000} {
		b.Run(fmt.Sprintf("results%d", count), func(b *testing.B) {
			s := searchFixture(b, count, 1)
			s.SearchByPath("/obj/search")
			for range 3 {
				searchTableFrame(s)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				searchTableFrame(s)
			}
			b.StopTimer()
			if len(s.results()) != count {
				b.Fatal("render discarded search results")
			}
			b.Logf("draw_sha256=%s", searchDrawHash())
			s.focusedResultIdx = count - 1
			var scroll, maximum float32
			for range 3 {
				scroll, maximum = searchTableFrame(s)
			}
			if scroll < maximum-100 {
				b.Fatal("final row not reachable")
			}
			b.Logf("bottom_draw_sha256=%s scroll=%.3f maximum=%.3f", searchDrawHash(), scroll, maximum)
		})
	}
}
