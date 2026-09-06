package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

type ConformanceFactory func() (SessionStore, error)

type ConformanceFixture struct {
	Initial       model.Snapshot
	Other         model.Snapshot
	First         model.AcceptedOperation
	FirstSnapshot model.Snapshot
	Second        model.AcceptedOperation
	Latest        model.Snapshot
}

func NewConformanceFixture() (ConformanceFixture, error) {
	initial, err := newConformanceSnapshot(2)
	if err != nil {
		return ConformanceFixture{}, err
	}
	other, err := newConformanceSnapshot(1)
	if err != nil {
		return ConformanceFixture{}, err
	}
	document, err := engine.NewDocument(initial)
	if err != nil {
		return ConformanceFixture{}, err
	}
	operation, err := newConformanceOperation(document.Snapshot(), 1)
	if err != nil {
		return ConformanceFixture{}, err
	}
	first, err := document.Apply(operation, time.Unix(1, 0).UTC())
	if err != nil {
		return ConformanceFixture{}, err
	}
	firstSnapshot := document.Snapshot()
	operation, err = newConformanceOperation(document.Snapshot(), 2)
	if err != nil {
		return ConformanceFixture{}, err
	}
	second, err := document.Apply(operation, time.Unix(2, 0).UTC())
	if err != nil {
		return ConformanceFixture{}, err
	}
	return ConformanceFixture{Initial: initial, Other: other, First: first, FirstSnapshot: firstSnapshot, Second: second, Latest: document.Snapshot()}, nil
}

func VerifyConformance(ctx context.Context, factory ConformanceFactory, fixture ConformanceFixture) error {
	if factory == nil {
		return fmt.Errorf("conformance factory is nil")
	}
	value, err := factory()
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	if value == nil {
		return fmt.Errorf("create store: returned nil")
	}
	defer func() { _ = value.Close() }()

	if err := value.Create(ctx, fixture.Initial); err != nil {
		return fmt.Errorf("create initial: %w", err)
	}
	if err := value.Create(ctx, fixture.Initial); !errors.Is(err, ErrSessionExists) {
		return fmt.Errorf("duplicate create error = %v, want %v", err, ErrSessionExists)
	}
	if err := value.Create(ctx, fixture.Other); err != nil {
		return fmt.Errorf("create other: %w", err)
	}
	checkpointID, err := model.NewCheckpointID()
	if err != nil {
		return fmt.Errorf("create checkpoint id: %w", err)
	}
	initialHash, err := fixture.Initial.Hash()
	if err != nil {
		return err
	}
	pendingCheckpoint := model.ExportCheckpoint{
		CheckpointID:   checkpointID,
		IdempotencyKey: "conformance-export-1",
		DocumentID:     fixture.Initial.DocumentID,
		SessionID:      "conformance-session",
		Revision:       fixture.Initial.Revision,
		MapHash:        initialHash,
		RequestedBy:    fixture.First.ActorID,
		CreatedAt:      time.Unix(3, 0).UTC(),
		Status:         model.ExportCheckpointPending,
	}
	createdCheckpoint, created, err := value.CreateExportCheckpoint(ctx, pendingCheckpoint)
	if err != nil || !created || !sameCheckpointState(createdCheckpoint, pendingCheckpoint) {
		return fmt.Errorf("create export checkpoint = %#v/%t/%v", createdCheckpoint, created, err)
	}
	retry := pendingCheckpoint
	retry.CheckpointID, err = model.NewCheckpointID()
	if err != nil {
		return err
	}
	retry.CreatedAt = time.Unix(4, 0).UTC()
	retriedCheckpoint, created, err := value.CreateExportCheckpoint(ctx, retry)
	if err != nil || created || !sameCheckpointState(retriedCheckpoint, pendingCheckpoint) {
		return fmt.Errorf("retry export checkpoint = %#v/%t/%v", retriedCheckpoint, created, err)
	}
	conflict := pendingCheckpoint
	conflict.Revision++
	if _, _, err := value.CreateExportCheckpoint(ctx, conflict); !errors.Is(err, ErrCheckpointConflict) {
		return fmt.Errorf("checkpoint idempotency conflict error = %v, want %v", err, ErrCheckpointConflict)
	}
	lookedUpCheckpoint, found, err := value.LookupExportCheckpoint(ctx, fixture.Initial.DocumentID, checkpointID)
	if err != nil || !found || !sameCheckpointState(lookedUpCheckpoint, pendingCheckpoint) {
		return fmt.Errorf("lookup export checkpoint = %#v/%t/%v", lookedUpCheckpoint, found, err)
	}
	completion := model.ExportCheckpointCompletion{
		Status:          model.ExportCheckpointAccepted,
		ArtifactHash:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Verifier:        "conformance-verifier",
		VerifierVersion: "1.0.0",
		CompletedAt:     time.Unix(5, 0).UTC(),
	}
	completedCheckpoint, err := value.CompleteExportCheckpoint(ctx, fixture.Initial.DocumentID, checkpointID, completion)
	if err != nil || completedCheckpoint.Status != model.ExportCheckpointAccepted {
		return fmt.Errorf("complete export checkpoint = %#v/%v", completedCheckpoint, err)
	}
	repeatedCheckpoint, err := value.CompleteExportCheckpoint(ctx, fixture.Initial.DocumentID, checkpointID, completion)
	if err != nil || !sameCheckpointState(repeatedCheckpoint, completedCheckpoint) {
		return fmt.Errorf("repeat export checkpoint completion = %#v/%v", repeatedCheckpoint, err)
	}

	loaded, operations, err := value.Load(ctx, fixture.Initial.DocumentID)
	if err != nil {
		return fmt.Errorf("load initial: %w", err)
	}
	if !reflect.DeepEqual(loaded, model.CloneSnapshot(fixture.Initial)) || len(operations) != 0 {
		return fmt.Errorf("initial load differs from created snapshot")
	}
	loaded.MaxX++
	loadedAgain, _, err := value.Load(ctx, fixture.Initial.DocumentID)
	if err != nil || !reflect.DeepEqual(loadedAgain, model.CloneSnapshot(fixture.Initial)) {
		return fmt.Errorf("snapshot load is not isolated")
	}

	if err := value.Append(ctx, fixture.Second); err == nil {
		return fmt.Errorf("non-contiguous append succeeded")
	}
	if err := value.Append(ctx, fixture.First); err != nil {
		return fmt.Errorf("append first: %w", err)
	}
	if err := value.Append(ctx, fixture.First); err != nil {
		return fmt.Errorf("idempotent append: %w", err)
	}
	if err := value.Append(ctx, fixture.Second); err != nil {
		return fmt.Errorf("append second: %w", err)
	}

	accepted, found, err := value.LookupOperation(ctx, fixture.Initial.DocumentID, fixture.First.OperationID)
	if err != nil || !found || !reflect.DeepEqual(accepted, fixture.First) {
		return fmt.Errorf("lookup first operation failed")
	}
	accepted.Operation.Changes[0].After.Prefabs[0].Path = "/mutated"
	acceptedAgain, found, err := value.LookupOperation(ctx, fixture.Initial.DocumentID, fixture.First.OperationID)
	if err != nil || !found || !reflect.DeepEqual(acceptedAgain, fixture.First) {
		return fmt.Errorf("operation lookup is not isolated")
	}

	base, replay, err := value.Load(ctx, fixture.Initial.DocumentID)
	if err != nil {
		return fmt.Errorf("load replay: %w", err)
	}
	if !reflect.DeepEqual(base, model.CloneSnapshot(fixture.Initial)) || len(replay) != 2 || !reflect.DeepEqual(replay[0], fixture.First) || !reflect.DeepEqual(replay[1], fixture.Second) {
		return fmt.Errorf("replay is incomplete or nondeterministic")
	}
	if err := value.SaveSnapshot(ctx, fixture.FirstSnapshot); err != nil {
		return fmt.Errorf("save retained snapshot: %w", err)
	}
	retained, replay, err := value.Load(ctx, fixture.Initial.DocumentID)
	if err != nil || !reflect.DeepEqual(retained, model.CloneSnapshot(fixture.FirstSnapshot)) || len(replay) != 1 || !reflect.DeepEqual(replay[0], fixture.Second) {
		return fmt.Errorf("retained snapshot discarded later replay")
	}
	if err := value.SaveSnapshot(ctx, fixture.Latest); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	compacted, replay, err := value.Load(ctx, fixture.Initial.DocumentID)
	if err != nil || !reflect.DeepEqual(compacted, model.CloneSnapshot(fixture.Latest)) || len(replay) != 0 {
		return fmt.Errorf("compacted load is incorrect")
	}
	if _, found, err := value.LookupOperation(ctx, fixture.Initial.DocumentID, fixture.First.OperationID); err != nil || !found {
		return fmt.Errorf("compaction discarded duplicate lookup history")
	}
	if retainedHash, found, err := value.RevisionHash(ctx, fixture.Initial.DocumentID, fixture.Initial.Revision); err != nil || !found || retainedHash != initialHash {
		return fmt.Errorf("compaction discarded initial revision hash")
	}
	if err := verifyRecoveryConformance(ctx, value, fixture); err != nil {
		return fmt.Errorf("snapshot recovery conformance: %w", err)
	}
	other, otherReplay, err := value.Load(ctx, fixture.Other.DocumentID)
	if err != nil || !reflect.DeepEqual(other, model.CloneSnapshot(fixture.Other)) || len(otherReplay) != 0 {
		return fmt.Errorf("documents are not isolated")
	}

	missingID := model.DocumentID("00000000-0000-4000-8000-000000000000")
	if _, _, err := value.Load(ctx, missingID); !errors.Is(err, ErrSessionMissing) {
		return fmt.Errorf("missing load error = %v, want %v", err, ErrSessionMissing)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := value.Load(canceled, fixture.Initial.DocumentID); !errors.Is(err, context.Canceled) {
		return fmt.Errorf("canceled load error = %v, want %v", err, context.Canceled)
	}
	if err := value.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err := value.Close(); err != nil {
		return fmt.Errorf("second close: %w", err)
	}
	if _, _, err := value.Load(ctx, fixture.Initial.DocumentID); !errors.Is(err, ErrStoreClosed) {
		return fmt.Errorf("load after close error = %v, want %v", err, ErrStoreClosed)
	}
	return nil
}

func newConformanceSnapshot(maxX int) (model.Snapshot, error) {
	documentID, err := model.NewDocumentID()
	if err != nil {
		return model.Snapshot{}, err
	}
	return model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      documentID,
		EnvironmentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MaxX:            maxX,
		MaxY:            1,
		MaxZ:            1,
	}, nil
}

func newConformanceOperation(snapshot model.Snapshot, x int) (model.Operation, error) {
	actorID, err := model.NewActorID()
	if err != nil {
		return model.Operation{}, err
	}
	operationID, err := model.NewOperationID()
	if err != nil {
		return model.Operation{}, err
	}
	stableID, err := model.NewStableID()
	if err != nil {
		return model.Operation{}, err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return model.Operation{}, err
	}
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         actorID,
		OperationID:     operationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     mapHash,
		Kind:            model.OperationKindTileChange,
		Changes: []model.TileChange{{
			Coord:  model.Coord{X: x, Y: 1, Z: 1},
			Before: model.TileState{},
			After: model.TileState{Prefabs: []model.PrefabState{{
				StableID: stableID,
				Path:     "/turf/open/floor",
				Vars:     map[string]string{},
			}}},
		}},
	}, nil
}
