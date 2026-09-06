package server

import (
	"context"
	"database/sql"
	"testing"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

func TestAuditRecoveryRetainsUndoAndStaleBases(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			initial := testSnapshot(t, 3)
			var value SessionStore = NewMemoryStore()
			var owner *DocumentOwner
			var err error
			if backend == "sqlite" {
				var sqliteOwner *DocumentOwner
				ctx, initial, value, sqliteOwner = auditRecoverySQLite(t)
				owner = sqliteOwner
			} else {
				owner, err = StartDocument(ctx, initial, value)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = value.Close() })
			}
			forward := testOperation(t, initial, 1)
			if _, err := owner.Submit(ctx, forward); err != nil {
				t.Fatal(err)
			}
			for cycle := 0; cycle < 2; cycle++ {
				current, err := owner.Snapshot(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := value.SaveSnapshot(ctx, current); err != nil {
					t.Fatal(err)
				}
				if err := owner.Close(ctx); err != nil {
					t.Fatal(err)
				}
				owner, err = RecoverDocument(ctx, initial.DocumentID, value)
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { _ = owner.Close(ctx) })
			id, err := model.NewOperationID()
			if err != nil {
				t.Fatal(err)
			}
			inverse, err := owner.BuildInverse(ctx, forward.ActorID, forward.OperationID, id)
			if err != nil {
				t.Fatalf("undo history lost: %v", err)
			}
			if _, err := owner.Submit(ctx, inverse); err != nil {
				t.Fatal(err)
			}
			if _, err := owner.Submit(ctx, testOperation(t, initial, 2)); err != nil {
				t.Fatalf("stale base lost: %v", err)
			}
			current, err := owner.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := value.SaveSnapshot(ctx, current); err != nil {
				t.Fatal(err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			owner, err = RecoverDocument(ctx, initial.DocumentID, value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.BuildInverse(ctx, forward.ActorID, forward.OperationID, id); engine.CodeOf(err) != engine.CodeAlreadyInverted {
				t.Fatalf("inverted status lost: %v", err)
			}
			redo := testOperation(t, current, 1)
			redo.ActorID = forward.ActorID
			if _, err := owner.Submit(ctx, redo); err != nil {
				t.Fatalf("redo: %v", err)
			}
		})
	}
}

func auditRecoverySQLite(t *testing.T) (context.Context, model.Snapshot, SessionStore, *DocumentOwner) {
	ctx, snapshot, value, owner, _ := auditSQLiteOwner(t)
	return ctx, snapshot, value, owner
}

func TestAuditSQLiteRejectsCorruptRecoveryRecords(t *testing.T) {
	for name, mutation := range map[string]string{
		"snapshot revision":  "UPDATE documents SET snapshot_revision = snapshot_revision + 1",
		"snapshot hash":      "UPDATE documents SET snapshot_hash = 'corrupt'",
		"operation identity": "UPDATE operations SET accepted = json_set(CAST(accepted AS TEXT), '$.document_id', '00000000-0000-7000-8000-000000000000') WHERE revision = 1",
		"operation revision": "UPDATE operations SET accepted = json_set(CAST(accepted AS TEXT), '$.revision', 4) WHERE revision = 1",
		"operation hash":     "UPDATE operations SET map_hash = 'corrupt' WHERE revision = 1",
		"missing middle":     "DELETE FROM operations WHERE revision = 1",
		"missing tail":       "DELETE FROM operations WHERE revision = 2",
		"revision hash":      "UPDATE revision_hashes SET map_hash = 'corrupt' WHERE revision = 1",
	} {
		t.Run(name, func(t *testing.T) {
			ctx, initial, value, owner, path := auditSQLiteOwner(t)
			for x := 1; x <= 2; x++ {
				if _, err := owner.Submit(ctx, testOperation(t, initial, x)); err != nil {
					t.Fatal(err)
				}
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			database, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = database.Close() }()
			if _, err := database.ExecContext(ctx, mutation); err != nil {
				t.Fatal(err)
			}
			recovered, err := RecoverDocument(ctx, initial.DocumentID, value)
			if err == nil {
				_ = recovered.Close(ctx)
				t.Fatal("corrupt durable state accepted")
			}
		})
	}
}
