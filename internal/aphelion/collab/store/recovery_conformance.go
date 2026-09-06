package store

import (
	"context"
	"fmt"
	"time"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

func verifyRecoveryConformance(ctx context.Context, value SessionStore, fixture ConformanceFixture) error {
	state, err := value.LoadRecovery(ctx, fixture.Initial.DocumentID)
	if err != nil {
		return err
	}
	document, err := state.Restore()
	if err != nil {
		return err
	}
	id, err := model.NewOperationID()
	if err != nil {
		return err
	}
	if _, err := document.BuildInverse(fixture.Second.ActorID, fixture.First.OperationID, id); engine.CodeOf(err) != engine.CodeActorMismatch {
		return fmt.Errorf("restored undo lost actor check: %v", err)
	}
	inverse, err := document.BuildInverse(fixture.First.ActorID, fixture.First.OperationID, id)
	if err != nil {
		return err
	}
	accepted, err := document.Apply(inverse, time.Unix(4, 0).UTC())
	if err != nil {
		return err
	}
	if err := value.Append(ctx, accepted); err != nil {
		return err
	}
	if err := value.SaveSnapshot(ctx, document.Snapshot()); err != nil {
		return err
	}
	state, err = value.LoadRecovery(ctx, fixture.Initial.DocumentID)
	if err != nil {
		return err
	}
	document, err = state.Restore()
	if err != nil {
		return err
	}
	if _, err := document.BuildInverse(fixture.First.ActorID, fixture.First.OperationID, id); engine.CodeOf(err) != engine.CodeAlreadyInverted {
		return fmt.Errorf("restored undo lost inverted status: %v", err)
	}
	// The reverted first tile admits a new edit based on the authentic initial
	// revision even though both snapshot boundaries are newer than that base.
	stale, err := newConformanceOperation(fixture.Initial, 1)
	if err != nil {
		return err
	}
	accepted, err = document.Apply(stale, time.Unix(5, 0).UTC())
	if err != nil {
		return err
	}
	if err := value.Append(ctx, accepted); err != nil {
		return err
	}
	state, err = value.LoadRecovery(ctx, fixture.Initial.DocumentID)
	if err != nil {
		return err
	}
	recovered, err := state.Restore()
	if err != nil {
		return err
	}
	gotHash, err := recovered.Snapshot().Hash()
	if err != nil {
		return err
	}
	wantHash, err := document.Snapshot().Hash()
	if err != nil {
		return err
	}
	if gotHash != wantHash || recovered.Snapshot().Revision != accepted.Revision {
		return fmt.Errorf("recovered tail differs from acknowledged state")
	}
	return nil
}
