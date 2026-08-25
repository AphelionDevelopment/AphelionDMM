package editor

import (
	"context"
	"fmt"
	"sort"

	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/executor"
	"sdmm/internal/aphelion/collab/mapadapter"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/app/command"
	"sdmm/internal/util"

	"github.com/rs/zerolog/log"
)

func (e *Editor) initializeCollaboration() {
	if e.documentID == "" {
		documentID, err := model.NewDocumentID()
		if err != nil {
			e.collaborationErr = fmt.Errorf("initialize collaboration document: %w", err)
			return
		}
		e.documentID = documentID
	}
	if e.actorID == "" {
		actorID, err := model.NewActorID()
		if err != nil {
			e.collaborationErr = fmt.Errorf("initialize collaboration actor: %w", err)
			return
		}
		e.actorID = actorID
	}
	environmentHash, err := mapadapter.EnvironmentHash(e.app.LoadedEnvironment())
	if err != nil {
		e.collaborationErr = err
		return
	}
	snapshot, err := mapadapter.Import(e.dmm, e.documentID, environmentHash)
	if err != nil {
		e.collaborationErr = err
		return
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		e.collaborationErr = err
		return
	}
	local, err := executor.NewLocal(document, e.actorID)
	if err != nil {
		e.collaborationErr = err
		return
	}
	e.executor = local
	e.collaborationErr = nil
	e.setAuthoritative(snapshot)
}

type projectionExecutor interface {
	executor.Executor
	ProjectionUpdates() <-chan client.Projection
}

type pendingExecutor interface {
	HasUnacknowledgedOperations() bool
}

// CollaborationSnapshot returns the committed map state used to start a collaboration session.
func (e *Editor) CollaborationSnapshot(ctx context.Context) (model.Snapshot, error) {
	if e.collaborationErr != nil {
		return model.Snapshot{}, fmt.Errorf("read collaboration snapshot: %w", e.collaborationErr)
	}
	if e.executor == nil {
		return model.Snapshot{}, fmt.Errorf("read collaboration snapshot: executor is unavailable")
	}
	if len(e.pendingChanges) != 0 {
		return model.Snapshot{}, fmt.Errorf("read collaboration snapshot: map has an uncommitted edit")
	}
	snapshot, err := e.executor.Snapshot(ctx)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("read collaboration snapshot: %w", err)
	}
	return model.CloneSnapshot(snapshot), nil
}

// AttachCollaborationExecutor replaces local compatibility mode with a synchronized executor.
func (e *Editor) AttachCollaborationExecutor(execution executor.Executor) error {
	if execution == nil {
		return fmt.Errorf("attach collaboration executor: executor is nil")
	}
	if len(e.pendingChanges) != 0 {
		return fmt.Errorf("attach collaboration executor: map has an uncommitted edit")
	}
	snapshot, err := execution.Snapshot(context.Background())
	if err != nil {
		return fmt.Errorf("attach collaboration executor: read snapshot: %w", err)
	}
	environmentHash, err := mapadapter.EnvironmentHash(e.app.LoadedEnvironment())
	if err != nil {
		return fmt.Errorf("attach collaboration executor: %w", err)
	}
	if snapshot.EnvironmentHash != environmentHash {
		return fmt.Errorf("attach collaboration executor: environment hash does not match the loaded project")
	}
	if err := mapadapter.ApplyWithEnvironment(e.dmm, snapshot, e.app.LoadedEnvironment()); err != nil {
		return fmt.Errorf("attach collaboration executor: %w", err)
	}
	e.executor = execution
	e.documentID = snapshot.DocumentID
	e.setAuthoritative(snapshot)
	e.refreshCollaborationView(e.pMap.ActiveLevel(), nil, snapshot)
	return nil
}

// DetachCollaborationExecutor restores local compatibility mode from the synchronized snapshot.
func (e *Editor) DetachCollaborationExecutor(ctx context.Context) error {
	if len(e.pendingChanges) != 0 {
		return fmt.Errorf("detach collaboration executor: map has an uncommitted edit")
	}
	if pending, ok := e.executor.(pendingExecutor); ok && pending.HasUnacknowledgedOperations() {
		return fmt.Errorf("detach collaboration executor: operations are awaiting acknowledgement")
	}
	snapshot, err := e.CollaborationSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("detach collaboration executor: %w", err)
	}
	document, err := engine.NewDocument(snapshot)
	if err != nil {
		return fmt.Errorf("detach collaboration executor: %w", err)
	}
	local, err := executor.NewLocal(document, e.actorID)
	if err != nil {
		return fmt.Errorf("detach collaboration executor: %w", err)
	}
	if err := mapadapter.ApplyWithEnvironment(e.dmm, snapshot, e.app.LoadedEnvironment()); err != nil {
		return fmt.Errorf("detach collaboration executor: %w", err)
	}
	e.executor = local
	e.setAuthoritative(snapshot)
	e.refreshCollaborationView(e.pMap.ActiveLevel(), nil, snapshot)
	return nil
}

// ProcessCollaborationUpdates applies the latest network projection on the UI thread.
func (e *Editor) ProcessCollaborationUpdates() {
	if len(e.pendingChanges) != 0 {
		return
	}
	execution, ok := e.executor.(projectionExecutor)
	if !ok {
		return
	}
	var latest *client.Projection
	for {
		select {
		case update := <-execution.ProjectionUpdates():
			projection := update
			latest = &projection
		default:
			if latest == nil {
				return
			}
			visible, err := latest.Visible()
			if err != nil {
				e.reportCollaborationError("Unable to project collaborative map", err)
				return
			}
			if err := mapadapter.ApplyWithEnvironment(e.dmm, visible, e.app.LoadedEnvironment()); err != nil {
				e.reportCollaborationError("Unable to project collaborative map", err)
				return
			}
			e.setAuthoritative(latest.Acknowledged)
			e.refreshCollaborationView(e.pMap.ActiveLevel(), nil, visible)
			return
		}
	}
}

// RefreshCollaborationSnapshot applies the executor's authoritative snapshot on the UI thread.
func (e *Editor) RefreshCollaborationSnapshot(ctx context.Context) error {
	if len(e.pendingChanges) != 0 {
		return fmt.Errorf("refresh collaboration snapshot: map has an uncommitted edit")
	}
	if e.executor == nil {
		return fmt.Errorf("refresh collaboration snapshot: executor is unavailable")
	}
	snapshot, err := e.executor.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("refresh collaboration snapshot: %w", err)
	}
	if err := mapadapter.ApplyWithEnvironment(e.dmm, snapshot, e.app.LoadedEnvironment()); err != nil {
		return fmt.Errorf("refresh collaboration snapshot: %w", err)
	}
	e.setAuthoritative(snapshot)
	e.refreshCollaborationView(e.pMap.ActiveLevel(), nil, snapshot)
	return nil
}

func (e *Editor) CanChangeMapSize() bool {
	_, asynchronous := e.executor.(executor.AsyncExecutor)
	return !asynchronous
}

func (e *Editor) BeginTileChange(point util.Point) {
	if e.collaborationErr != nil || e.executor == nil {
		return
	}
	coord := model.Coord{X: point.X, Y: point.Y, Z: point.Z}
	if _, exists := e.pendingChanges[coord]; exists {
		return
	}
	before, err := mapadapter.CaptureTile(e.dmm.GetTile(point))
	if err != nil {
		e.collaborationErr = err
		log.Error().Err(err).Msg("Unable to capture map change")
		return
	}
	e.pendingChanges[coord] = before
}

func (e *Editor) commitOperation(commitMessage string) {
	if len(e.pendingChanges) == 0 {
		return
	}
	execution := e.executor
	base, err := execution.Snapshot(context.Background())
	if err != nil {
		e.rejectSpeculation(execution, fmt.Errorf("read authoritative snapshot: %w", err))
		return
	}
	coords := make([]model.Coord, 0, len(e.pendingChanges))
	for coord := range e.pendingChanges {
		coords = append(coords, coord)
	}
	sort.Slice(coords, func(left int, right int) bool {
		if coords[left].Z != coords[right].Z {
			return coords[left].Z < coords[right].Z
		}
		if coords[left].Y != coords[right].Y {
			return coords[left].Y < coords[right].Y
		}
		return coords[left].X < coords[right].X
	})
	changes := make([]model.TileChange, 0, len(coords))
	for _, coord := range coords {
		point := util.Point{X: coord.X, Y: coord.Y, Z: coord.Z}
		after, captureErr := mapadapter.CaptureTile(e.dmm.GetTile(point))
		if captureErr != nil {
			e.rejectSpeculation(execution, captureErr)
			return
		}
		before := e.pendingChanges[coord]
		if before.Equal(after) {
			continue
		}
		changes = append(changes, model.TileChange{Coord: coord, Before: before, After: after})
	}
	e.pendingChanges = make(map[model.Coord]model.TileState)
	if len(changes) == 0 {
		return
	}
	operation, err := e.forwardOperation(base, changes)
	if err != nil {
		e.rejectSpeculation(execution, err)
		return
	}
	acceptedChanges := model.CloneOperation(operation).Changes
	activeLevel := e.pMap.ActiveLevel()
	e.submitOperation(execution, operation, func(accepted model.AcceptedOperation) {
		e.syncFromExecutor(execution, false, activeLevel, coords)
		e.pushAcceptedCommand(execution, commitMessage, accepted, acceptedChanges, activeLevel, coords)
	})
}

func (e *Editor) submitOperation(execution executor.Executor, operation model.Operation, accepted func(model.AcceptedOperation)) {
	if asynchronous, ok := execution.(executor.AsyncExecutor); ok {
		err := asynchronous.ExecuteAsync(context.Background(), operation, func(result model.AcceptedOperation, executeErr error) {
			e.app.RunLater(func() {
				if executeErr != nil {
					e.rejectSpeculation(execution, executeErr)
					return
				}
				accepted(result)
			})
		})
		if err != nil {
			e.rejectSpeculation(execution, err)
		}
		return
	}
	result, err := execution.Execute(context.Background(), operation)
	if err != nil {
		e.rejectSpeculation(execution, err)
		return
	}
	accepted(result)
}

func (e *Editor) pushAcceptedCommand(execution executor.Executor, commitMessage string, accepted model.AcceptedOperation, acceptedChanges []model.TileChange, activeLevel int, coords []model.Coord) {
	forwardID := accepted.OperationID
	e.app.CommandStorage().Push(command.MakeAsync(commitMessage, func(complete func(error)) {
		inverse, inverseErr := execution.BuildInverse(context.Background(), forwardID)
		if inverseErr != nil {
			e.reportCollaborationError("Unable to undo map change", inverseErr)
			complete(inverseErr)
			return
		}
		e.executeHistoryOperation(execution, inverse, func(_ model.AcceptedOperation, executeErr error) {
			if executeErr != nil {
				e.reportCollaborationError("Unable to undo map change", executeErr)
				complete(executeErr)
				return
			}
			e.syncFromExecutor(execution, true, activeLevel, coords)
			complete(nil)
		})
	}, func(complete func(error)) {
		current, redoErr := execution.Snapshot(context.Background())
		if redoErr != nil {
			e.reportCollaborationError("Unable to redo map change", redoErr)
			complete(redoErr)
			return
		}
		redo, redoErr := e.forwardOperation(current, acceptedChanges)
		if redoErr != nil {
			e.reportCollaborationError("Unable to redo map change", redoErr)
			complete(redoErr)
			return
		}
		e.executeHistoryOperation(execution, redo, func(redone model.AcceptedOperation, executeErr error) {
			if executeErr != nil {
				e.reportCollaborationError("Unable to redo map change", executeErr)
				complete(executeErr)
				return
			}
			forwardID = redone.OperationID
			e.syncFromExecutor(execution, true, activeLevel, coords)
			complete(nil)
		})
	}))
}

func (e *Editor) executeHistoryOperation(execution executor.Executor, operation model.Operation, complete func(model.AcceptedOperation, error)) {
	if asynchronous, ok := execution.(executor.AsyncExecutor); ok {
		err := asynchronous.ExecuteAsync(context.Background(), operation, func(result model.AcceptedOperation, executeErr error) {
			e.app.RunLater(func() {
				complete(result, executeErr)
			})
		})
		if err != nil {
			complete(model.AcceptedOperation{}, err)
		}
		return
	}
	result, err := execution.Execute(context.Background(), operation)
	complete(result, err)
}

func (e *Editor) forwardOperation(snapshot model.Snapshot, changes []model.TileChange) (model.Operation, error) {
	operationID, err := model.NewOperationID()
	if err != nil {
		return model.Operation{}, fmt.Errorf("create operation id: %w", err)
	}
	baseHash, err := snapshot.Hash()
	if err != nil {
		return model.Operation{}, fmt.Errorf("hash operation base: %w", err)
	}
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      snapshot.DocumentID,
		ActorID:         e.actorID,
		OperationID:     operationID,
		BaseRevision:    snapshot.Revision,
		EnvironmentHash: snapshot.EnvironmentHash,
		BaseMapHash:     baseHash,
		Kind:            model.OperationKindTileChange,
		Changes:         changes,
	}, nil
}

func (e *Editor) rejectSpeculation(execution executor.Executor, cause error) {
	e.pendingChanges = make(map[model.Coord]model.TileState)
	e.syncFromExecutor(execution, true, e.pMap.ActiveLevel(), nil)
	e.reportCollaborationError("Unable to apply map change", cause)
}

func (e *Editor) syncFromExecutor(execution executor.Executor, apply bool, activeLevel int, coords []model.Coord) {
	snapshot, err := execution.Snapshot(context.Background())
	if err != nil {
		e.reportCollaborationError("Unable to synchronize map", err)
		return
	}
	if apply {
		if err := mapadapter.ApplyWithEnvironment(e.dmm, snapshot, e.app.LoadedEnvironment()); err != nil {
			e.reportCollaborationError("Unable to synchronize map", err)
			return
		}
	}
	e.executor = execution
	e.setAuthoritative(snapshot)
	e.refreshCollaborationView(activeLevel, coords, snapshot)
}

func (e *Editor) refreshCollaborationView(activeLevel int, coords []model.Coord, visible model.Snapshot) {
	e.pMap.Snapshot().Sync()
	e.updateAreasZones()
	points := make([]util.Point, 0, len(coords))
	for _, coord := range coords {
		points = append(points, util.Point{X: coord.X, Y: coord.Y, Z: coord.Z})
	}
	if len(points) == 0 {
		for _, tile := range visible.Tiles {
			points = append(points, util.Point{X: tile.Coord.X, Y: tile.Coord.Y, Z: tile.Coord.Z})
		}
	}
	e.updateBucket(activeLevel, points)
	e.dmm.PersistPrefabs()
	e.app.SyncPrefabs()
	e.app.SyncVarEditor()
}

func (e *Editor) setAuthoritative(snapshot model.Snapshot) {
	e.authoritative = model.CloneSnapshot(snapshot)
	e.authoritativeTiles = make(map[model.Coord]model.TileState, len(snapshot.Tiles))
	for _, tile := range snapshot.Tiles {
		e.authoritativeTiles[tile.Coord] = model.CloneTileState(tile.State)
	}
	e.pendingChanges = make(map[model.Coord]model.TileState)
}

func (e *Editor) reportCollaborationError(message string, err error) {
	log.Error().Err(err).Msg(message)
	util.ShowErrorDialog(message + ": " + err.Error())
}
