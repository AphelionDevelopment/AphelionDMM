package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestStoreConformance(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "collaboration.db")
	if err := collabstore.VerifyConformance(context.Background(), func() (collabstore.SessionStore, error) {
		return Open(path)
	}, fixture); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSQLiteVersionMeetsMinimum(t *testing.T) {
	t.Parallel()

	value, err := Open(filepath.Join(t.TempDir(), "version.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if compareVersions(value.Version(), minimumSQLiteVersion) < 0 {
		t.Fatalf("SQLite version = %s, want at least %s", value.Version(), minimumSQLiteVersion)
	}
	t.Logf("SQLite runtime version: %s", value.Version())
}

func TestStoreSurvivesCloseAndReopen(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "restart.db")
	value, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	_, replay, err := reopened.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].OperationID != fixture.First.OperationID {
		t.Fatalf("replay after reopen = %#v, want operation %q", replay, fixture.First.OperationID)
	}
}

func TestOpenMigratesRevisionHashesFromSchemaOne(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema-one.db")
	value, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	if _, err := value.database.Exec("DROP TABLE revision_hashes"); err != nil {
		t.Fatal(err)
	}
	if _, err := value.database.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	initialHash, err := fixture.Initial.Hash()
	if err != nil {
		t.Fatal(err)
	}
	firstHash, err := fixture.FirstSnapshot.Hash()
	if err != nil {
		t.Fatal(err)
	}
	for revision, expected := range map[model.Revision]string{
		fixture.Initial.Revision: initialHash,
		fixture.First.Revision:   firstHash,
	} {
		actual, found, err := reopened.RevisionHash(context.Background(), fixture.Initial.DocumentID, revision)
		if err != nil || !found || actual != expected {
			t.Fatalf("RevisionHash(%d) = %q, %t, %v; want %q", revision, actual, found, err, expected)
		}
	}
}

func TestConcurrentDuplicateAppendIsIdempotent(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(filepath.Join(t.TempDir(), "duplicate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}

	const workers = 16
	errorsFound := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := value.Append(context.Background(), fixture.First); err != nil {
				errorsFound <- err
			}
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("Append() error = %v", err)
	}
	_, replay, err := value.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 {
		t.Fatalf("replay length = %d, want 1", len(replay))
	}
}

func TestOpenRejectsFutureSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "future.db")
	value, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.database.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open() error = nil for future schema")
	}
}

func TestLockedDatabaseHonorsContextAndRollsBack(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "locked.db")
	value, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	locker, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = locker.Close() })
	connection, err := locker.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if _, err := connection.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = value.Create(ctx, fixture.Initial)
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "database is locked")) {
		t.Fatalf("Create() error = %v, want deadline or locked error", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("locked Create() elapsed = %v, want at most 1s", elapsed)
	}
	if _, err := connection.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatalf("Create() after rollback: %v", err)
	}
}

func TestAppendFailureRollsBackTransaction(t *testing.T) {
	t.Parallel()

	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if _, err := value.database.Exec(`CREATE TRIGGER fail_operation BEFORE INSERT ON operations BEGIN SELECT RAISE(ABORT, 'injected append failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err == nil {
		t.Fatal("Append() error = nil with failure trigger")
	}
	if _, err := value.database.Exec("DROP TRIGGER fail_operation"); err != nil {
		t.Fatal(err)
	}
	_, replay, err := value.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 0 {
		t.Fatalf("replay after failed append = %#v, want empty", replay)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatalf("Append() after rollback: %v", err)
	}
}
