package servicehost

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCloudflaredArgumentsPassOnlyTokenFilePath(t *testing.T) {
	tokenPath := filepath.Join("C:\\ProgramData", "AphelionDMM", "Relay", "cloudflare_tunnel_token.txt")
	require.Equal(t, []string{"--no-autoupdate", "tunnel", "run", "--token-file", tokenPath}, CloudflaredArguments(tokenPath))
}

func TestHostWaitsForRelayReadinessBeforeStartingTunnel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	readyChecked := make(chan struct{})
	tunnelStarted := make(chan struct{})
	host := Host{
		Relay: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		WaitReady: func(context.Context) error {
			close(readyChecked)
			return nil
		},
		Tunnel: func(ctx context.Context) error {
			select {
			case <-readyChecked:
			default:
				t.Fatal("tunnel started before relay readiness completed")
			}
			close(tunnelStarted)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx) }()
	require.Eventually(t, func() bool {
		select {
		case <-tunnelStarted:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}

func TestHostCancelsRelayWhenTunnelExitsUnexpectedly(t *testing.T) {
	relayStopped := make(chan struct{})
	host := Host{
		Relay: func(ctx context.Context) error {
			<-ctx.Done()
			close(relayStopped)
			return ctx.Err()
		},
		WaitReady: func(context.Context) error { return nil },
		Tunnel:    func(context.Context) error { return errors.New("connector exited") },
	}

	err := host.Run(context.Background())
	require.ErrorContains(t, err, "connector exited")
	select {
	case <-relayStopped:
	case <-time.After(time.Second):
		t.Fatal("relay was not canceled after connector exit")
	}
}

func TestHostRejectsUnexpectedCleanTunnelExit(t *testing.T) {
	host := Host{
		Relay: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		WaitReady: func(context.Context) error { return nil },
		Tunnel:    func(context.Context) error { return nil },
	}

	require.EqualError(t, host.Run(context.Background()), "tunnel stopped without an error")
}

func TestHostDoesNotStartTunnelWhenReadinessFails(t *testing.T) {
	relayStopped := make(chan struct{})
	tunnelStarted := false
	host := Host{
		Relay: func(ctx context.Context) error {
			<-ctx.Done()
			close(relayStopped)
			return ctx.Err()
		},
		WaitReady: func(context.Context) error { return errors.New("relay never became ready") },
		Tunnel: func(context.Context) error {
			tunnelStarted = true
			return nil
		},
	}

	err := host.Run(context.Background())
	require.ErrorContains(t, err, "relay never became ready")
	require.False(t, tunnelStarted)
	select {
	case <-relayStopped:
	case <-time.After(time.Second):
		t.Fatal("relay was not canceled after readiness failure")
	}
}
