package relayruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

// Run serves a configured relay until cancellation or failure.
func Run(ctx context.Context, config relay.Config, revision string, output io.Writer) error {
	service := relay.NewService(config)
	server := &http.Server{
		Addr:              config.BindAddress,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	_, _ = fmt.Fprintf(output, "apheliondmm-relay bind=%s protocol=%d revision=%s\n", config.BindAddress, protocolv2.Version, revision)
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve relay: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shut down relay: %w", err)
		}
		return ctx.Err()
	}
}
