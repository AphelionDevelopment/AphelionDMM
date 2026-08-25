package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
	"sdmm/internal/aphelion/collab/store/sqlite"
)

func TestStoreConformance(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	if err := collabstore.VerifyConformance(context.Background(), func() (collabstore.SessionStore, error) {
		return Open(context.Background(), Config{DSN: dsn, Schema: schema})
	}, fixture); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentStoresSerializeDocumentRevision(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	first, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if err := first.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}

	const workers = 16
	results := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		value := first
		if index%2 != 0 {
			value = second
		}
		go func() {
			defer group.Done()
			results <- value.Append(context.Background(), fixture.First)
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("concurrent append: %v", err)
		}
	}
	if err := second.Append(context.Background(), fixture.Second); err != nil {
		t.Fatal(err)
	}
	_, replay, err := first.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 2 || replay[0].Revision != 1 || replay[1].Revision != 2 {
		t.Fatalf("serialized replay = %#v", replay)
	}
}

func TestAppendHonorsCancellationWhileAnotherInstanceLocksDocument(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema, StatementTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close(context.Background()) })
	if _, err := connection.Exec(context.Background(), "SET search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	transaction, err := connection.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(context.Background(), `SELECT 1 FROM collaboration_documents WHERE document_id = $1 FOR UPDATE`, fixture.Initial.DocumentID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	err = value.Append(ctx, fixture.First)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("locked append error = %v, want deadline", err)
	}
	if err := transaction.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatalf("append after rollback: %v", err)
	}
}

func TestMigrationContentionIsSerialized(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	results := make(chan error, 2)
	stores := make(chan *Store, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
			if err == nil {
				stores <- value
			}
			results <- err
		}()
	}
	group.Wait()
	close(results)
	close(stores)
	for err := range results {
		if err != nil {
			t.Errorf("contended migration: %v", err)
		}
	}
	for value := range stores {
		_ = value.Close()
	}
}

func TestPoolRecoversAfterDatabaseConnectionTermination(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema, MaxConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	var backendPID uint32
	if err := value.pool.QueryRow(context.Background(), "SELECT pg_backend_pid()").Scan(&backendPID); err != nil {
		t.Fatal(err)
	}
	terminator, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = terminator.Close(context.Background()) }()
	var terminated bool
	if err := terminator.QueryRow(context.Background(), "SELECT pg_terminate_backend($1)", backendPID).Scan(&terminated); err != nil {
		t.Fatal(err)
	}
	if !terminated {
		t.Fatal("PostgreSQL backend was not terminated")
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatalf("append after connection termination: %v", err)
	}
}

func TestOperationIDIsUniqueAcrossDocuments(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := value.Create(context.Background(), fixture.Other); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	otherOperation := model.CloneOperation(fixture.First.Operation)
	otherOperation.DocumentID = fixture.Other.DocumentID
	otherOperation.EnvironmentHash = fixture.Other.EnvironmentHash
	otherOperation.BaseRevision = fixture.Other.Revision
	otherOperation.BaseMapHash, err = fixture.Other.Hash()
	if err != nil {
		t.Fatal(err)
	}
	otherDocument, err := engine.NewDocument(fixture.Other)
	if err != nil {
		t.Fatal(err)
	}
	otherAccepted, err := otherDocument.Apply(otherOperation, fixture.First.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Append(context.Background(), otherAccepted); err == nil {
		t.Fatal("operation ID collision across documents was accepted")
	}
	_, replay, err := value.Load(context.Background(), fixture.Other.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 0 {
		t.Fatalf("failed operation ID collision changed other document: %#v", replay)
	}
}

func TestPostgreSQLMatchesSQLiteRecovery(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	postgresValue, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = postgresValue.Close() })
	sqliteValue, err := sqlite.Open(filepath.Join(t.TempDir(), "comparison.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteValue.Close() })
	for _, value := range []collabstore.SessionStore{postgresValue, sqliteValue} {
		if err := value.Create(context.Background(), fixture.Initial); err != nil {
			t.Fatal(err)
		}
		if err := value.Append(context.Background(), fixture.First); err != nil {
			t.Fatal(err)
		}
		if err := value.Append(context.Background(), fixture.Second); err != nil {
			t.Fatal(err)
		}
		if err := value.SaveSnapshot(context.Background(), fixture.FirstSnapshot); err != nil {
			t.Fatal(err)
		}
	}
	postgresSnapshot, postgresReplay, err := postgresValue.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	sqliteSnapshot, sqliteReplay, err := sqliteValue.Load(context.Background(), fixture.Initial.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(postgresSnapshot, sqliteSnapshot) || !reflect.DeepEqual(postgresReplay, sqliteReplay) {
		t.Fatal("PostgreSQL and SQLite recovered different state")
	}
	for revision := model.Revision(0); revision <= fixture.Second.Revision; revision++ {
		postgresHash, postgresFound, postgresErr := postgresValue.RevisionHash(context.Background(), fixture.Initial.DocumentID, revision)
		sqliteHash, sqliteFound, sqliteErr := sqliteValue.RevisionHash(context.Background(), fixture.Initial.DocumentID, revision)
		if postgresErr != nil || sqliteErr != nil || postgresFound != sqliteFound || postgresHash != sqliteHash {
			t.Fatalf("revision %d hashes differ: PostgreSQL %q/%t/%v SQLite %q/%t/%v", revision, postgresHash, postgresFound, postgresErr, sqliteHash, sqliteFound, sqliteErr)
		}
	}
}

func TestRetryClassification(t *testing.T) {
	t.Parallel()

	for _, code := range []string{"40001", "40P01", "08006", "57P03"} {
		if !IsRetryable(&pgconn.PgError{Code: code}) {
			t.Errorf("code %s was not retryable", code)
		}
	}
	if IsRetryable(&pgconn.PgError{Code: "23505"}) || IsRetryable(context.Canceled) {
		t.Fatal("permanent error was classified as retryable")
	}
}

func isolatedSchema(t *testing.T) (string, string) {
	t.Helper()
	dsn := os.Getenv("APHELION_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("APHELION_POSTGRES_TEST_DSN is not configured")
	}
	id, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "aphelion_test_" + strings.ReplaceAll(string(id), "-", "")
	connection, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(context.Background(), "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = connection.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = connection.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		_ = connection.Close(context.Background())
	})
	return dsn, schema
}
