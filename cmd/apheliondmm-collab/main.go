package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/server"
	"sdmm/internal/aphelion/collab/store/sqlite"
)

const maxSnapshotConfigBytes = 256 << 20

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("apheliondmm-collab", flag.ContinueOnError)
	flags.SetOutput(stderr)
	snapshotPath := flags.String("snapshot-config", "", "trusted local collaboration snapshot JSON")
	listenAddress := flags.String("listen", "127.0.0.1:8080", "loopback listen address")
	launchTokenPath := flags.String("launch-token-file", "", "new protected file that receives the single-use launch token")
	databasePath := flags.String("database", "", "trusted local SQLite collaboration database; memory-only when omitted")
	snapshotOperationThreshold := flags.Int("snapshot-operation-threshold", 0, "persist a compact snapshot after this many accepted operations; disabled when zero")
	snapshotInterval := flags.Duration("snapshot-interval", 0, "maximum interval between durable snapshots; disabled when zero")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if *snapshotPath == "" || *launchTokenPath == "" {
		_, _ = fmt.Fprintln(stderr, "-snapshot-config and -launch-token-file are required")
		return 2
	}
	if *snapshotOperationThreshold < 0 || *snapshotInterval < 0 {
		_, _ = fmt.Fprintln(stderr, "-snapshot-operation-threshold and -snapshot-interval cannot be negative")
		return 2
	}
	snapshot, err := loadSnapshot(*snapshotPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "load snapshot config: %v\n", err)
		return 1
	}
	var durableStore server.SessionStore
	if *databasePath != "" {
		durableStore, err = sqlite.Open(*databasePath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "open collaboration database: %v\n", err)
			return 1
		}
	}
	embedded, err := server.StartEmbeddedWithConfig(ctx, snapshot, server.EmbeddedConfig{
		Store:         durableStore,
		ListenAddress: *listenAddress,
		Document: server.DocumentConfig{
			SnapshotOperationThreshold: *snapshotOperationThreshold,
			SnapshotInterval:           *snapshotInterval,
		},
	})
	if err != nil {
		if durableStore != nil {
			_ = durableStore.Close()
		}
		_, _ = fmt.Fprintf(stderr, "start collaboration service: %v\n", err)
		return 1
	}
	launchToken := embedded.TakeLaunchToken()
	if err := writeLaunchToken(*launchTokenPath, launchToken); err != nil {
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		_ = embedded.Shutdown(shutdownContext)
		cancelShutdown()
		_, _ = fmt.Fprintf(stderr, "write launch token: %v\n", err)
		return 1
	}
	defer func() { _ = os.Remove(*launchTokenPath) }()
	_, _ = fmt.Fprintf(stdout, "base_url=%q\n", embedded.BaseURL)
	<-ctx.Done()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := embedded.Shutdown(shutdownContext); err != nil {
		_, _ = fmt.Fprintf(stderr, "shutdown collaboration service: %v\n", err)
		return 1
	}
	return 0
}

func loadSnapshot(path string) (model.Snapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return model.Snapshot{}, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return model.Snapshot{}, err
	}
	if info.Size() > maxSnapshotConfigBytes {
		return model.Snapshot{}, fmt.Errorf("snapshot config is %d bytes, maximum is %d", info.Size(), maxSnapshotConfigBytes)
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxSnapshotConfigBytes+1))
	decoder.DisallowUnknownFields()
	var snapshot model.Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return model.Snapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return model.Snapshot{}, fmt.Errorf("snapshot config contains trailing JSON")
	}
	if _, err := snapshot.Hash(); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func writeLaunchToken(path, token string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	writeErr := func() error {
		if _, err := io.WriteString(file, token); err != nil {
			return err
		}
		return file.Sync()
	}()
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	return closeErr
}
