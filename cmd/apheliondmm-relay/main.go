package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relay"
)

var buildRevision = "development"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("apheliondmm-relay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "strict relay YAML configuration")
	checkConfig := flags.Bool("check-config", false, "validate configuration and exit")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse relay arguments: %w", err)
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("positional arguments are not accepted")
	}
	config, err := relay.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if *checkConfig {
		_, _ = fmt.Fprintln(output, "apheliondmm-relay configuration valid")
		return nil
	}
	service := relay.NewService(config)
	server := &http.Server{
		Addr:              config.BindAddress,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	_, _ = fmt.Fprintf(output, "apheliondmm-relay bind=%s protocol=%d revision=%s\n", config.BindAddress, protocolv2.Version, buildRevision)
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
		return nil
	}
}
