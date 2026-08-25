package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"sdmm/internal/aphelion/collab/model"
)

type Embedded struct {
	BaseURL string

	listener    net.Listener
	service     *Service
	server      *http.Server
	tokenMutex  sync.Mutex
	launchToken string
	stopOnce    sync.Once
	stopErr     error
}

func StartEmbedded(ctx context.Context, snapshot model.Snapshot) (*Embedded, error) {
	return StartEmbeddedWithConfig(ctx, snapshot, EmbeddedConfig{})
}

func StartEmbeddedWithConfig(ctx context.Context, snapshot model.Snapshot, config EmbeddedConfig) (*Embedded, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := snapshot.Hash(); err != nil {
		return nil, fmt.Errorf("validate embedded snapshot: %w", err)
	}
	config = config.withDefaults()
	if err := ValidateListenAddress(config.ListenAddress, false); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen for embedded collaboration: %w", err)
	}
	baseURL := "http://" + listener.Addr().String()
	service := NewService(ServiceConfig{AllowedOrigins: []string{baseURL, "http://127.0.0.1"}})
	launchToken, err := service.NewLaunchTokenForSnapshot(snapshot)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	httpServer := &http.Server{
		Handler:           service.Handler(),
		ReadHeaderTimeout: config.ReadTimeout,
		ReadTimeout:       config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
	}
	embedded := &Embedded{
		BaseURL: baseURL, launchToken: launchToken,
		listener: listener, service: service, server: httpServer,
	}
	go func() {
		serveErr := httpServer.Serve(listener)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && service.config.OnWebSocketError != nil {
			service.config.OnWebSocketError(serveErr)
		}
	}()
	return embedded, nil
}

func (embedded *Embedded) Endpoint() string {
	return embedded.BaseURL
}

func (embedded *Embedded) TakeLaunchToken() string {
	embedded.tokenMutex.Lock()
	defer embedded.tokenMutex.Unlock()
	token := embedded.launchToken
	embedded.launchToken = ""
	return token
}

func (embedded *Embedded) Shutdown(ctx context.Context) error {
	embedded.stopOnce.Do(func() {
		if err := embedded.server.Shutdown(ctx); err != nil {
			embedded.stopErr = err
		}
		if err := embedded.service.Shutdown(ctx); embedded.stopErr == nil && err != nil {
			embedded.stopErr = err
		}
	})
	return embedded.stopErr
}
