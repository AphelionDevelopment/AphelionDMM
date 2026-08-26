package authority

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/store"
)

func TestDocumentCommitsBeforeAdvancingAndTreatsDuplicatesAsIdempotent(t *testing.T) {
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
	value := store.NewMemoryStore()
	document, err := StartDocument(context.Background(), fixture.Initial, value)
	require.NoError(t, err)
	t.Cleanup(func() { _ = document.Close(context.Background()) })

	accepted, duplicate, err := document.Submit(context.Background(), fixture.First.Operation)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.Equal(t, fixture.First.Operation, accepted.Operation)
	acceptedAgain, duplicate, err := document.Submit(context.Background(), fixture.First.Operation)
	require.NoError(t, err)
	require.True(t, duplicate)
	require.Equal(t, accepted, acceptedAgain)
}

func TestDocumentDoesNotAdvanceWhenAppendFails(t *testing.T) {
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
	value := &appendFailingStore{SessionStore: store.NewMemoryStore()}
	document, err := StartDocument(context.Background(), fixture.Initial, value)
	require.NoError(t, err)
	t.Cleanup(func() { _ = document.Close(context.Background()) })

	_, _, err = document.Submit(context.Background(), fixture.First.Operation)
	require.ErrorIs(t, err, errInjectedAppend)
	snapshot, err := document.Snapshot(context.Background())
	require.NoError(t, err)
	require.Equal(t, fixture.Initial.Revision, snapshot.Revision)
	wantHash, err := fixture.Initial.Hash()
	require.NoError(t, err)
	actualHash, err := snapshot.Hash()
	require.NoError(t, err)
	require.Equal(t, wantHash, actualHash)
}

type appendFailingStore struct {
	store.SessionStore
}

var errInjectedAppend = errors.New("injected append failure")

func (value *appendFailingStore) Append(context.Context, model.AcceptedOperation) error {
	return errInjectedAppend
}
