package store

import (
	"context"
	"testing"
)

func TestMemoryStoreConformance(t *testing.T) {
	t.Parallel()

	fixture, err := NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConformance(context.Background(), func() (SessionStore, error) {
		return NewMemoryStore(), nil
	}, fixture); err != nil {
		t.Fatal(err)
	}
}
