package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

func TestReconnectPolicyUsesBoundedBackoffAndAcknowledgedRevision(t *testing.T) {
	t.Parallel()

	var delays []time.Duration
	var revisions []model.Revision
	policy := ReconnectPolicy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     250 * time.Millisecond,
		Jitter:       0.25,
		MaxAttempts:  4,
		Random:       func() float64 { return 1 },
		Wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	}
	attempts := 0
	err := policy.Reconnect(context.Background(), 7, func(_ context.Context, revision model.Revision) error {
		attempts++
		revisions = append(revisions, revision)
		if attempts < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(delays) != 2 || delays[0] != 125*time.Millisecond || delays[1] != 250*time.Millisecond {
		t.Fatalf("delays = %v", delays)
	}
	for _, revision := range revisions {
		if revision != 7 {
			t.Fatalf("resume revision = %d, want 7", revision)
		}
	}
}

func TestReconnectPolicyStopsOnPermanentAndContextErrors(t *testing.T) {
	t.Parallel()

	policy := ReconnectPolicy{Wait: func(context.Context, time.Duration) error { return nil }}
	attempts := 0
	err := policy.Reconnect(context.Background(), 0, func(context.Context, model.Revision) error {
		attempts++
		return PermanentReconnectError(ErrAuthenticationDenied)
	})
	if !errors.Is(err, ErrAuthenticationDenied) || attempts != 1 {
		t.Fatalf("permanent reconnect = %v after %d attempts", err, attempts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := policy.Reconnect(ctx, 0, func(context.Context, model.Revision) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconnect = %v", err)
	}
}
