package load

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

func TestRelayRunnerUsesOnlyClientSuppliedBundle(t *testing.T) {
	for _, participants := range []int{2, 8, 32} {
		t.Run(fmt.Sprintf("participants_%d", participants), func(t *testing.T) {
			service := relay.NewService(loadRelayTestConfig())
			server := httptest.NewServer(service.Handler())
			t.Cleanup(server.Close)
			bundle := relayTestBundle(t, server.URL, participants)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, err := RunRelay(ctx, bundle, 4)
			require.NoError(t, err)
			require.Equal(t, participants, result.Clients)
			require.Equal(t, participants*4, result.Messages)
			require.Positive(t, result.MessagesPerSecond)
		})
	}
}

func relayTestBundle(t *testing.T, relayURL string, participants int) RelayBundle {
	t.Helper()
	_, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	room := make([]byte, 16)
	group := make([]byte, 32)
	_, err = rand.Read(room)
	require.NoError(t, err)
	_, err = rand.Read(group)
	require.NoError(t, err)
	bundle := RelayBundle{RelayURL: relayURL, RoomID: base64.RawURLEncoding.EncodeToString(room), OwnerPrivateKey: base64.RawURLEncoding.EncodeToString(ownerPrivate), GroupKey: base64.RawURLEncoding.EncodeToString(group)}
	for range participants {
		_, privateKey, keyErr := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, keyErr)
		admission := make([]byte, 32)
		_, keyErr = rand.Read(admission)
		require.NoError(t, keyErr)
		bundle.Participants = append(bundle.Participants, RelayBundleParticipant{PrivateKey: base64.RawURLEncoding.EncodeToString(privateKey), Admission: base64.RawURLEncoding.EncodeToString(admission), Role: protocolv2.RoleEditor})
	}
	return bundle
}

func loadRelayTestConfig() relay.Config {
	return relay.Config{
		Version: 1, PublicOrigin: "https://mapping.a13.info", BindAddress: "127.0.0.1:0", RoomIdleTTL: relay.Duration(time.Minute),
		Limits:        relay.Limits{MaxRooms: 10, MaxConnections: 100, MaxConnectionsPerRoom: 65, MaxMessageBytes: 1 << 20, MaxRoomBytesPerSecond: 64 << 20, ConnectBurst: 200, ConnectWindow: relay.Duration(time.Minute), MessageBurst: 10000, MessageWindow: relay.Duration(time.Second)},
		Observability: relay.ObservabilityConfig{LogLevel: "info", MetricsBindAddress: "127.0.0.1:0"},
	}
}
