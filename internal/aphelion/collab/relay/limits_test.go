package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWindowLimiterEnforcesBudgetAndBoundsKeys(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	limiter := newWindowLimiter(2, time.Minute, 2)
	require.True(t, limiter.Allow("first", 1, now))
	require.True(t, limiter.Allow("first", 1, now))
	require.False(t, limiter.Allow("first", 1, now))
	require.True(t, limiter.Allow("second", 1, now))
	require.False(t, limiter.Allow("third", 1, now))
	require.True(t, limiter.Allow("third", 2, now.Add(time.Minute)))
}

func TestWindowLimiterRejectsOversizedSingleCost(t *testing.T) {
	limiter := newWindowLimiter(10, time.Second, 1)
	require.False(t, limiter.Allow("room", 11, time.Unix(100, 0).UTC()))
}
