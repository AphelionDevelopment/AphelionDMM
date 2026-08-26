package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"sdmm/internal/aphelion/collab/identity"
	sqlitestore "sdmm/internal/aphelion/collab/store/sqlite"
)

type clientCollaboration struct {
	internalDir string
	store       *sqlitestore.Store
	manager     *identity.Manager
	identity    identity.Identity
}

func initializeClientCollaboration(internalDir string, config *collaborationConfig) (*clientCollaboration, error) {
	if config == nil {
		return nil, fmt.Errorf("collaboration config is unavailable")
	}
	directory := filepath.Join(internalDir, "collaboration")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create collaboration data directory: %w", err)
	}
	secretStore, err := identity.NewPlatformStore(filepath.Join(directory, "secrets"))
	if err != nil {
		return nil, fmt.Errorf("initialize collaboration secret store: %w", err)
	}
	manager := identity.NewManager(secretStore, nil)
	localIdentity, reference, err := manager.LoadOrCreate(context.Background(), identity.SecretRef(config.IdentitySecretRef))
	if err != nil {
		return nil, fmt.Errorf("initialize collaboration identity: %w", err)
	}
	database, err := sqlitestore.Open(filepath.Join(directory, "collaboration.db"))
	if err != nil {
		return nil, fmt.Errorf("initialize collaboration database: %w", err)
	}
	config.IdentitySecretRef = string(reference)
	return &clientCollaboration{internalDir: internalDir, store: database, manager: manager, identity: localIdentity}, nil
}
