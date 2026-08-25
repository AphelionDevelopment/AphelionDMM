package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestCorruptCopiedDatabaseFailsWithoutChangingSource(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := source.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(directory, "corrupt.db")
	copy(data[:16], []byte("not sqlite data!"))
	if err := os.WriteFile(corruptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if corrupt, err := Open(corruptPath); err == nil {
		defer corrupt.Close()
		if _, _, loadErr := corrupt.Load(context.Background(), fixture.Initial.DocumentID); loadErr == nil {
			t.Fatal("corrupt copied database opened and loaded")
		}
	}
	reopened, err := Open(sourcePath)
	if err != nil {
		t.Fatalf("open untouched source: %v", err)
	}
	defer reopened.Close()
	_, replay, err := reopened.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatalf("load untouched source: %v", err)
	}
	if len(replay) != 1 || replay[0].OperationID != fixture.First.OperationID {
		t.Fatalf("untouched source replay = %#v", replay)
	}
}
