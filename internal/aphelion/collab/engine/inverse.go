package engine

import (
	"fmt"

	"sdmm/internal/aphelion/collab/model"
)

func (document *Document) BuildInverse(actor model.ActorID, targetID model.OperationID, inverseID model.OperationID) (model.Operation, error) {
	if err := actor.Validate(); err != nil {
		return model.Operation{}, document.reject(CodeInvalidOperation, err)
	}
	if err := inverseID.Validate(); err != nil {
		return model.Operation{}, document.reject(CodeInvalidOperation, err)
	}
	target, exists := document.accepted[targetID]
	if !exists {
		return model.Operation{}, document.reject(CodeOperationNotFound, fmt.Errorf("target operation %q was not accepted", targetID))
	}
	if target.ActorID != actor {
		return model.Operation{}, document.reject(CodeActorMismatch, fmt.Errorf("target operation belongs to actor %q", target.ActorID))
	}
	if inverse, exists := document.inverted[targetID]; exists {
		return model.Operation{}, document.reject(CodeAlreadyInverted, fmt.Errorf("target operation was inverted by %q", inverse))
	}
	if target.Kind == model.OperationKindInverse {
		return model.Operation{}, document.reject(CodeInvalidOperation, fmt.Errorf("inverse operations are redone as new forward operations"))
	}

	changes := make([]model.TileChange, len(target.Changes))
	tileIndexes := make(map[model.Coord]int, len(document.snapshot.Tiles))
	for index, tile := range document.snapshot.Tiles {
		tileIndexes[tile.Coord] = index
	}
	for index, targetChange := range target.Changes {
		currentIndex, exists := tileIndexes[targetChange.Coord]
		current := model.TileState{}
		if exists {
			current = document.snapshot.Tiles[currentIndex].State
		}
		if !current.Equal(targetChange.After) {
			return model.Operation{}, document.reject(CodePreconditionFailed, fmt.Errorf("target after-value changed at (%d,%d,%d)", targetChange.Coord.X, targetChange.Coord.Y, targetChange.Coord.Z))
		}
		changes[index] = model.TileChange{
			Coord:  targetChange.Coord,
			Before: model.CloneTileState(targetChange.After),
			After:  model.CloneTileState(targetChange.Before),
		}
	}

	inverseOf := targetID
	return model.Operation{
		ProtocolVersion: model.ProtocolVersion,
		DocumentID:      document.snapshot.DocumentID,
		ActorID:         actor,
		OperationID:     inverseID,
		BaseRevision:    document.snapshot.Revision,
		EnvironmentHash: document.snapshot.EnvironmentHash,
		BaseMapHash:     document.mapHash,
		Kind:            model.OperationKindInverse,
		Changes:         changes,
		InverseOf:       &inverseOf,
	}, nil
}

func (document *Document) validateInverse(operation model.Operation) error {
	if operation.Kind != model.OperationKindInverse {
		return nil
	}
	if operation.InverseOf == nil {
		return document.reject(CodeInvalidOperation, fmt.Errorf("inverse operation must name its target"))
	}
	target, exists := document.accepted[*operation.InverseOf]
	if !exists {
		return document.reject(CodeOperationNotFound, fmt.Errorf("target operation %q was not accepted", *operation.InverseOf))
	}
	if target.ActorID != operation.ActorID {
		return document.reject(CodeActorMismatch, fmt.Errorf("target operation belongs to actor %q", target.ActorID))
	}
	if target.Kind == model.OperationKindInverse {
		return document.reject(CodeInvalidOperation, fmt.Errorf("inverse operations are redone as new forward operations"))
	}
	if inverse, exists := document.inverted[*operation.InverseOf]; exists {
		return document.reject(CodeAlreadyInverted, fmt.Errorf("target operation was inverted by %q", inverse))
	}
	if len(operation.Changes) != len(target.Changes) {
		return document.reject(CodeInvalidOperation, fmt.Errorf("inverse change count is %d, want %d", len(operation.Changes), len(target.Changes)))
	}
	for index, change := range operation.Changes {
		targetChange := target.Changes[index]
		if change.Coord != targetChange.Coord || !change.Before.Equal(targetChange.After) || !change.After.Equal(targetChange.Before) {
			return document.reject(CodeInvalidOperation, fmt.Errorf("inverse change %d does not exactly reverse target", index))
		}
	}
	return nil
}
