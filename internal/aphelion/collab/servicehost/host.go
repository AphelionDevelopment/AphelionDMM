package servicehost

import (
	"context"
	"errors"
	"fmt"
)

// Component runs until its context is canceled or it fails.
type Component func(context.Context) error

// Host coordinates the relay and its optional edge connector.
type Host struct {
	Relay     Component
	WaitReady Component
	Tunnel    Component
}

// Run starts the relay, waits for readiness, then supervises the connector.
func (host Host) Run(ctx context.Context) error {
	if host.Relay == nil || host.WaitReady == nil {
		return fmt.Errorf("relay and readiness components are required")
	}
	componentContext, cancel := context.WithCancel(ctx)
	defer cancel()
	relayErrors := make(chan error, 1)
	go func() { relayErrors <- host.Relay(componentContext) }()
	readyErrors := make(chan error, 1)
	go func() { readyErrors <- host.WaitReady(componentContext) }()

	select {
	case <-ctx.Done():
		cancel()
		<-relayErrors
		return nil
	case err := <-relayErrors:
		cancel()
		if cleanComponentExit(ctx, err) {
			return nil
		}
		return componentStoppedError("relay stopped before readiness", err)
	case err := <-readyErrors:
		if err != nil {
			cancel()
			<-relayErrors
			return fmt.Errorf("wait for relay readiness: %w", err)
		}
	}

	if host.Tunnel == nil {
		select {
		case <-ctx.Done():
			cancel()
			<-relayErrors
			return nil
		case err := <-relayErrors:
			if cleanComponentExit(ctx, err) {
				return nil
			}
			return componentStoppedError("relay stopped", err)
		}
	}

	tunnelErrors := make(chan error, 1)
	go func() { tunnelErrors <- host.Tunnel(componentContext) }()
	select {
	case <-ctx.Done():
		cancel()
		<-relayErrors
		<-tunnelErrors
		return nil
	case err := <-relayErrors:
		cancel()
		<-tunnelErrors
		if cleanComponentExit(ctx, err) {
			return nil
		}
		return componentStoppedError("relay stopped", err)
	case err := <-tunnelErrors:
		cancel()
		<-relayErrors
		if cleanComponentExit(ctx, err) {
			return nil
		}
		return componentStoppedError("tunnel stopped", err)
	}
}

func componentStoppedError(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%s without an error", message)
	}
	return fmt.Errorf("%s: %w", message, err)
}

func cleanComponentExit(parent context.Context, err error) bool {
	return parent.Err() != nil && (err == nil || errors.Is(err, context.Canceled) || errors.Is(err, parent.Err()))
}
