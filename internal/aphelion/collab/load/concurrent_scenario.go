package load

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"reflect"

	"sdmm/internal/aphelion/collab/model"
)

type ConcurrentConfig struct {
	Config
	ConflictPairs int `json:"conflict_pairs"`
}

type ConcurrentIntent struct {
	Client    int             `json:"client"`
	Operation model.Operation `json:"operation"`
}

type ConcurrentScenario struct {
	Config           ConcurrentConfig   `json:"config"`
	Initial          model.Snapshot     `json:"initial"`
	Actors           []model.ActorID    `json:"actors"`
	Intents          []ConcurrentIntent `json:"intents"`
	ExpectedAccepted int                `json:"expected_accepted"`
	ExpectedRejected int                `json:"expected_rejected"`
	ExpectedMapHash  string             `json:"expected_map_hash"`
}

// GenerateConcurrent creates independent revision-zero intents. Each conflict
// pair requests identical content with different operation IDs, so exactly one
// wins and the final map hash is independent of network arrival order.
func GenerateConcurrent(config ConcurrentConfig) (ConcurrentScenario, error) {
	if err := validateConfig(config.Config); err != nil {
		return ConcurrentScenario{}, err
	}
	if config.ConflictPairs < 0 || config.ConflictPairs > config.Operations/2 ||
		(config.ConflictPairs > 0 && config.Clients < 2) || config.Operations-config.ConflictPairs > config.MaxX*config.MaxY {
		return ConcurrentScenario{}, fmt.Errorf("concurrent scenario needs distinct cells for independent intents and two clients per conflict pair")
	}
	initial := model.Snapshot{ProtocolVersion: model.ProtocolVersion, SchemaVersion: model.SchemaVersion,
		DocumentID:      model.DocumentID(deterministicUUID(config.Seed, "concurrent-document", 0)),
		EnvironmentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("environment:%d", config.Seed)))),
		MaxX:            config.MaxX, MaxY: config.MaxY, MaxZ: 1}
	baseHash, err := initial.Hash()
	if err != nil {
		return ConcurrentScenario{}, err
	}
	result := ConcurrentScenario{Config: config, Initial: initial, ExpectedAccepted: config.Operations - config.ConflictPairs, ExpectedRejected: config.ConflictPairs}
	for i := 0; i < config.Clients; i++ {
		result.Actors = append(result.Actors, model.ActorID(deterministicUUID(config.Seed, "concurrent-actor", i)))
	}
	positions := rand.New(rand.NewSource(config.Seed)).Perm(config.MaxX * config.MaxY)
	final := model.CloneSnapshot(initial)
	for group := 0; group < result.ExpectedAccepted; group++ {
		position := positions[group]
		final.Tiles = append(final.Tiles, model.Tile{Coord: model.Coord{X: position%config.MaxX + 1, Y: position/config.MaxX + 1, Z: 1},
			State: model.TileState{Prefabs: []model.PrefabState{{StableID: model.StableID(deterministicUUID(config.Seed, "concurrent-prefab", group)), Path: "/turf/open/floor", Vars: map[string]string{}}}}})
	}
	for i := 0; i < config.Operations; i++ {
		group := i - config.ConflictPairs
		if i < 2*config.ConflictPairs {
			group = i / 2
		}
		tile := final.Tiles[group]
		actorIndex := i % len(result.Actors)
		result.Intents = append(result.Intents, ConcurrentIntent{Client: actorIndex, Operation: model.Operation{
			ProtocolVersion: model.ProtocolVersion, DocumentID: initial.DocumentID, ActorID: result.Actors[actorIndex],
			OperationID:  model.OperationID(deterministicUUID(config.Seed, "concurrent-operation", i)),
			BaseRevision: 0, BaseMapHash: baseHash, EnvironmentHash: initial.EnvironmentHash, Kind: model.OperationKindTileChange,
			Changes: []model.TileChange{{Coord: tile.Coord, After: model.CloneTileState(tile.State)}},
		}})
	}
	result.ExpectedMapHash, err = final.Hash()
	return result, err
}

// Verify rejects altered counts, schedules or intents before any network work.
// Randomized engine application in tests independently checks the generator.
func (scenario ConcurrentScenario) Verify() error {
	expected, err := GenerateConcurrent(scenario.Config)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(scenario, expected) {
		return fmt.Errorf("concurrent scenario differs from its deterministic manifest")
	}
	return nil
}
