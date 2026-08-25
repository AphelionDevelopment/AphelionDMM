package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestLogicalBackupRestoresExactRevisionAndHashBeforeAndAfterOperation(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	binaryDirectory := os.Getenv("APHELION_POSTGRES_BIN")
	if binaryDirectory == "" {
		t.Skip("APHELION_POSTGRES_BIN is not configured")
	}
	pgDump := filepath.Join(binaryDirectory, postgresExecutable(runtime.GOOS, "pg_dump"))
	pgRestore := filepath.Join(binaryDirectory, postgresExecutable(runtime.GOOS, "pg_restore"))
	for _, path := range []string{pgDump, pgRestore} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("PostgreSQL backup tool %q is unavailable", path)
		}
	}
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	source, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if err := source.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	if err := source.Append(context.Background(), fixture.First); err != nil {
		t.Fatal(err)
	}
	before := restoreLogicalBackup(t, pgDump, pgRestore, dsn, schema, fixture.Initial.DocumentID)
	if before.revision != fixture.First.Revision {
		t.Fatalf("pre-operation restored revision = %d, want %d", before.revision, fixture.First.Revision)
	}
	if err := source.Append(context.Background(), fixture.Second); err != nil {
		t.Fatal(err)
	}
	after := restoreLogicalBackup(t, pgDump, pgRestore, dsn, schema, fixture.Initial.DocumentID)
	if after.revision != fixture.Second.Revision || after.mapHash == before.mapHash {
		t.Fatalf("post-operation restore = %#v, pre-operation = %#v", after, before)
	}
	t.Logf("restore before: duration=%s artifact_sha256=%s revision=%d map_hash=%s", before.duration, before.artifactHash, before.revision, before.mapHash)
	t.Logf("restore after: duration=%s artifact_sha256=%s revision=%d map_hash=%s", after.duration, after.artifactHash, after.revision, after.mapHash)
}

func TestPostgresExecutableUsesPlatformSuffix(t *testing.T) {
	if value := postgresExecutable("windows", "pg_dump"); value != "pg_dump.exe" {
		t.Fatalf("Windows executable = %q", value)
	}
	if value := postgresExecutable("linux", "pg_dump"); value != "pg_dump" {
		t.Fatalf("Linux executable = %q", value)
	}
}

func postgresExecutable(goos, name string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

type restoreEvidence struct {
	duration     time.Duration
	artifactHash string
	revision     model.Revision
	mapHash      string
}

func restoreLogicalBackup(t *testing.T, pgDump, pgRestore, dsn, schema string, documentID model.DocumentID) restoreEvidence {
	t.Helper()
	started := time.Now()
	archive := filepath.Join(t.TempDir(), "collaboration.dump")
	sourceEnvironment, err := libpqEnvironment(dsn, "")
	if err != nil {
		t.Fatal(err)
	}
	runPostgresTool(t, pgDump, sourceEnvironment, "--schema="+schema, "--format=custom", "--no-owner", "--no-privileges", "--file="+archive)
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	artifactHashBytes := sha256.Sum256(archiveData)

	configuration, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	id, err := model.NewOperationID()
	if err != nil {
		t.Fatal(err)
	}
	databaseName := "aphelion_restore_" + strings.ReplaceAll(string(id), "-", "")
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{databaseName}.Sanitize()); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	targetConfiguration := configuration.Copy()
	targetConfiguration.Database = databaseName
	targetDSN := targetConfiguration.ConnString()
	targetEnvironment, err := libpqEnvironment(dsn, databaseName)
	if err != nil {
		t.Fatal(err)
	}
	runPostgresTool(t, pgRestore, targetEnvironment, "--dbname="+databaseName, "--no-owner", "--no-privileges", "--exit-on-error", archive)

	restored, err := Open(context.Background(), Config{DSN: targetDSN, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	base, replay, err := restored.Load(context.Background(), documentID)
	if err != nil {
		_ = restored.Close()
		t.Fatal(err)
	}
	document, err := engine.NewDocument(base)
	if err != nil {
		_ = restored.Close()
		t.Fatal(err)
	}
	for _, accepted := range replay {
		if _, err := document.Apply(accepted.Operation, accepted.AcceptedAt); err != nil {
			_ = restored.Close()
			t.Fatal(err)
		}
	}
	snapshot := document.Snapshot()
	mapHash, err := snapshot.Hash()
	if err != nil {
		_ = restored.Close()
		t.Fatal(err)
	}
	storedHash, found, err := restored.RevisionHash(context.Background(), documentID, snapshot.Revision)
	if closeErr := restored.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil || !found || storedHash != mapHash {
		t.Fatalf("restored revision hash = %q/%t/%v, reconstructed = %q", storedHash, found, err, mapHash)
	}
	return restoreEvidence{duration: time.Since(started), artifactHash: hex.EncodeToString(artifactHashBytes[:]), revision: snapshot.Revision, mapHash: mapHash}
}

func runPostgresTool(t *testing.T, executable string, environment []string, arguments ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v: %s", filepath.Base(executable), err, boundedToolOutput(output))
	}
}

func libpqEnvironment(dsn, database string) ([]string, error) {
	configuration, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if database == "" {
		database = configuration.Database
	}
	sslMode := "require"
	if configuration.TLSConfig == nil {
		sslMode = "disable"
	}
	return append(os.Environ(),
		"PGHOST="+configuration.Host,
		fmt.Sprintf("PGPORT=%d", configuration.Port),
		"PGDATABASE="+database,
		"PGUSER="+configuration.User,
		"PGPASSWORD="+configuration.Password,
		"PGSSLMODE="+sslMode,
	), nil
}

func boundedToolOutput(output []byte) string {
	const maximum = 4096
	if len(output) > maximum {
		return fmt.Sprintf("%s...", output[:maximum])
	}
	return string(output)
}
