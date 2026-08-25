package dmmdata

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"
)

func TestSaveAtomicPreservesOriginalOnWriteFailure(t *testing.T) {
	t.Parallel()

	target := existingTarget(t)
	wantErr := errors.New("write failed")
	err := SaveAtomic(target, func(writer io.Writer) error {
		if _, writeErr := io.WriteString(writer, "partial replacement"); writeErr != nil {
			return writeErr
		}
		return wantErr
	}, func(string) error {
		t.Fatal("validator called after failed write")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SaveAtomic() error = %v, want %v", err, wantErr)
	}
	assertOriginalAndNoStages(t, target)
}

func TestSaveAtomicPreservesOriginalOnValidationFailure(t *testing.T) {
	t.Parallel()

	target := existingTarget(t)
	wantErr := errors.New("validation failed")
	err := SaveAtomic(target, func(writer io.Writer) error {
		_, err := io.WriteString(writer, "complete replacement")
		return err
	}, func(stagedPath string) error {
		contents, readErr := os.ReadFile(stagedPath)
		if readErr != nil {
			t.Fatalf("read staged file: %v", readErr)
		}
		if string(contents) != "complete replacement" {
			t.Fatalf("staged contents = %q", contents)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SaveAtomic() error = %v, want %v", err, wantErr)
	}
	assertOriginalAndNoStages(t, target)
}

func TestSaveAtomicReplacesValidatedTarget(t *testing.T) {
	t.Parallel()

	target := existingTarget(t)
	err := SaveAtomic(target, func(writer io.Writer) error {
		_, err := io.WriteString(writer, "complete replacement")
		return err
	}, func(stagedPath string) error {
		_, err := os.Stat(stagedPath)
		return err
	})
	if err != nil {
		t.Fatalf("SaveAtomic() error = %v", err)
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(contents) != "complete replacement" {
		t.Fatalf("target contents = %q, want complete replacement", contents)
	}
	assertNoStages(t, target)
}

func TestWriteDMPropagatesWriterFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("writer failed")
	err := fixtureData("").WriteDM(&failingWriter{remaining: 8, err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WriteDM() error = %v, want %v", err, wantErr)
	}
}

func TestSaveDMReparsesSemanticMap(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "map.dmm")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("create original target: %v", err)
	}
	data := fixtureData(target)
	if err := data.SaveDM(target); err != nil {
		t.Fatalf("SaveDM() error = %v", err)
	}
	reparsed, err := New(target)
	if err != nil {
		t.Fatalf("reparse saved map: %v", err)
	}
	wantDigest, err := data.semanticDigest()
	if err != nil {
		t.Fatalf("digest source: %v", err)
	}
	gotDigest, err := reparsed.semanticDigest()
	if err != nil {
		t.Fatalf("digest reparsed map: %v", err)
	}
	if gotDigest != wantDigest {
		t.Fatalf("reparsed digest = %x, want %x", gotDigest, wantDigest)
	}
}

type failingWriter struct {
	remaining int
	err       error
}

func (writer *failingWriter) Write(data []byte) (int, error) {
	if writer.remaining <= 0 {
		return 0, writer.err
	}
	if len(data) > writer.remaining {
		written := writer.remaining
		writer.remaining = 0
		return written, writer.err
	}
	writer.remaining -= len(data)
	return len(data), nil
}

func existingTarget(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "map.dmm")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("create original target: %v", err)
	}
	return target
}

func assertOriginalAndNoStages(t *testing.T, target string) {
	t.Helper()
	contents, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read original target: %v", err)
	}
	if string(contents) != "original" {
		t.Fatalf("original target changed to %q", contents)
	}
	assertNoStages(t, target)
}

func assertNoStages(t *testing.T, target string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*"))
	if err != nil {
		t.Fatalf("glob staging files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("staging files remain: %v", matches)
	}
}

func fixtureData(path string) DmmData {
	variables := &dmvars.MutableVariables{}
	variables.Put("dir", "2")
	prefabs := Prefabs{
		dmmprefab.New(dmmprefab.IdNone, "/area/foo", (&dmvars.MutableVariables{}).ToImmutable()),
		dmmprefab.New(dmmprefab.IdNone, "/turf/foo", (&dmvars.MutableVariables{}).ToImmutable()),
		dmmprefab.New(dmmprefab.IdNone, "/obj/foo1", variables.ToImmutable()),
	}
	return DmmData{
		Filepath:   path,
		LineBreak:  "\n",
		KeyLength:  1,
		MaxX:       1,
		MaxY:       1,
		MaxZ:       1,
		Dictionary: DataDictionary{"a": prefabs},
		Grid:       DataGrid{util.Point{X: 1, Y: 1, Z: 1}: "a"},
	}
}
