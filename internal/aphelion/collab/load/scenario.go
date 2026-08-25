package load

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"github.com/google/uuid"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
)

type Config struct {
	Seed                                  int64 `json:"seed"`
	Clients                               int   `json:"clients"`
	Operations                            int   `json:"operations"`
	PresencePerClient                     int   `json:"presence_per_client"`
	MaxX                                  int   `json:"max_x"`
	MaxY                                  int   `json:"max_y"`
	TargetOperationsPerSecond             int   `json:"target_operations_per_second,omitempty"`
	TargetPresencePerSecondPerEditor      int   `json:"target_presence_per_second_per_editor,omitempty"`
	MaximumP95AcknowledgementMilliseconds int   `json:"maximum_p95_acknowledgement_ms,omitempty"`
}

type PresenceUpdate struct {
	ActorID  model.ActorID `json:"actor_id"`
	Sequence uint64        `json:"sequence"`
	Cursor   model.Coord   `json:"cursor"`
}

type Scenario struct {
	Config           Config            `json:"config"`
	Initial          model.Snapshot    `json:"initial"`
	Actors           []model.ActorID   `json:"actors"`
	Operations       []model.Operation `json:"operations"`
	Presence         []PresenceUpdate  `json:"presence"`
	ExpectedRevision model.Revision    `json:"expected_revision"`
	ExpectedMapHash  string            `json:"expected_map_hash"`
}

func Generate(config Config) (Scenario, error) {
	if err := validateConfig(config); err != nil {
		return Scenario{}, err
	}
	initial := model.Snapshot{
		ProtocolVersion: model.ProtocolVersion,
		SchemaVersion:   model.SchemaVersion,
		DocumentID:      model.DocumentID(deterministicUUID(config.Seed, "document", 0)),
		EnvironmentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("environment:%d", config.Seed)))),
		MaxX:            config.MaxX, MaxY: config.MaxY, MaxZ: 1,
	}
	document, err := engine.NewDocument(initial)
	if err != nil {
		return Scenario{}, err
	}
	actors := make([]model.ActorID, config.Clients)
	for index := range actors {
		actors[index] = model.ActorID(deterministicUUID(config.Seed, "actor", index))
	}
	random := rand.New(rand.NewSource(config.Seed))
	operations := make([]model.Operation, 0, config.Operations)
	states := make(map[model.Coord]model.TileState)
	for index := 0; index < config.Operations; index++ {
		coord := model.Coord{X: random.Intn(config.MaxX) + 1, Y: random.Intn(config.MaxY) + 1, Z: 1}
		before := model.CloneTileState(states[coord])
		path := "/turf/open/floor"
		if index%2 != 0 {
			path = "/turf/open/floor/plating"
		}
		after := model.TileState{Prefabs: []model.PrefabState{{StableID: model.StableID(deterministicUUID(config.Seed, "prefab", index)), Path: path, Vars: map[string]string{}}}}
		base := document.Snapshot()
		baseHash, hashErr := base.Hash()
		if hashErr != nil {
			return Scenario{}, hashErr
		}
		operation := model.Operation{
			ProtocolVersion: model.ProtocolVersion, DocumentID: initial.DocumentID, ActorID: actors[index%len(actors)], OperationID: model.OperationID(deterministicUUID(config.Seed, "operation", index)),
			BaseRevision: base.Revision, EnvironmentHash: initial.EnvironmentHash, BaseMapHash: baseHash, Kind: model.OperationKindTileChange,
			Changes: []model.TileChange{{Coord: coord, Before: before, After: after}},
		}
		if _, err := document.Apply(operation, time.Unix(int64(index+1), 0).UTC()); err != nil {
			return Scenario{}, err
		}
		states[coord] = model.CloneTileState(after)
		operations = append(operations, operation)
	}
	presence := make([]PresenceUpdate, 0, config.Clients*config.PresencePerClient)
	for sequence := 1; sequence <= config.PresencePerClient; sequence++ {
		for actorIndex, actorID := range actors {
			presence = append(presence, PresenceUpdate{ActorID: actorID, Sequence: uint64(sequence), Cursor: model.Coord{X: (actorIndex+sequence)%config.MaxX + 1, Y: (actorIndex*2+sequence)%config.MaxY + 1, Z: 1}})
		}
	}
	final := document.Snapshot()
	finalHash, err := final.Hash()
	if err != nil {
		return Scenario{}, err
	}
	return Scenario{Config: config, Initial: initial, Actors: actors, Operations: operations, Presence: presence, ExpectedRevision: final.Revision, ExpectedMapHash: finalHash}, nil
}

func (scenario Scenario) Verify() error {
	if err := validateConfig(scenario.Config); err != nil {
		return err
	}
	if len(scenario.Actors) != scenario.Config.Clients || len(scenario.Operations) != scenario.Config.Operations || len(scenario.Presence) != scenario.Config.Clients*scenario.Config.PresencePerClient {
		return fmt.Errorf("load scenario counts do not match its bounded configuration")
	}
	document, err := engine.NewDocument(scenario.Initial)
	if err != nil {
		return err
	}
	for index, operation := range scenario.Operations {
		accepted, err := document.Apply(operation, time.Unix(int64(index+1), 0).UTC())
		if err != nil {
			return fmt.Errorf("apply scenario operation %d: %w", index, err)
		}
		if !reflect.DeepEqual(accepted.Operation, operation) {
			return fmt.Errorf("scenario operation %d changed during replay", index)
		}
	}
	final := document.Snapshot()
	hash, err := final.Hash()
	if err != nil {
		return err
	}
	if final.Revision != scenario.ExpectedRevision || hash != scenario.ExpectedMapHash {
		return fmt.Errorf("scenario result is revision %d hash %s, want revision %d hash %s", final.Revision, hash, scenario.ExpectedRevision, scenario.ExpectedMapHash)
	}
	return nil
}

func validateConfig(config Config) error {
	if config.Seed == 0 || config.Clients < 1 || config.Clients > 100 || config.Operations < 1 || config.Operations > 100000 || config.PresencePerClient < 0 || config.PresencePerClient > 1000 || config.MaxX < 1 || config.MaxX > 1000 || config.MaxY < 1 || config.MaxY > 1000 {
		return fmt.Errorf("load scenario configuration is outside bounded limits")
	}
	if config.TargetOperationsPerSecond < 0 || config.TargetOperationsPerSecond > 10000 || config.TargetPresencePerSecondPerEditor < 0 || config.TargetPresencePerSecondPerEditor > 10000 || config.MaximumP95AcknowledgementMilliseconds < 0 || config.MaximumP95AcknowledgementMilliseconds > 600000 {
		return fmt.Errorf("load scenario performance targets are outside bounded limits")
	}
	return nil
}

func deterministicUUID(seed int64, kind string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%d", seed, kind, index)))
	var value uuid.UUID
	copy(value[:], sum[:len(value)])
	value[6] = value[6]&0x0f | 0x70
	value[8] = value[8]&0x3f | 0x80
	return value.String()
}
