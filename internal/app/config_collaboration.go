package app

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"sdmm/internal/aphelion/collab/relayclient"
)

const (
	collaborationConfigName    = "collaboration"
	collaborationConfigVersion = 1
	defaultRelayURL            = "https://mapping.a13.info"
)

type collaborationConfig struct {
	Version           uint
	RelayURL          string
	IdentitySecretRef string `json:",omitempty"`
}

func newCollaborationConfig() *collaborationConfig {
	return &collaborationConfig{Version: collaborationConfigVersion, RelayURL: defaultRelayURL}
}

func (collaborationConfig) Name() string {
	return collaborationConfigName
}

func (collaborationConfig) TryMigrate(raw map[string]any) (map[string]any, bool) {
	version, _ := raw["Version"].(float64)
	if uint(version) >= collaborationConfigVersion {
		return nil, false
	}
	result := map[string]any{"Version": collaborationConfigVersion, "RelayURL": defaultRelayURL}
	if relayURL, ok := raw["RelayURL"].(string); ok && relayURL != "" {
		result["RelayURL"] = relayURL
	}
	return result, true
}

func (config collaborationConfig) Validate() error {
	if config.Version != collaborationConfigVersion {
		return fmt.Errorf("collaboration config version %d is unsupported", config.Version)
	}
	if err := relayclient.ValidateEndpoint(config.RelayURL); err != nil {
		return fmt.Errorf("collaboration relay URL: %w", err)
	}
	return nil
}

func (a *app) loadCollaborationConfig() {
	config := newCollaborationConfig()
	a.ConfigRegister(config)
	if err := config.Validate(); err != nil {
		log.Error().Err(err).Msg("invalid collaboration config; restoring public relay default")
		*config = *newCollaborationConfig()
	}
}

func (a *app) collaborationConfig() *collaborationConfig {
	if config, ok := a.ConfigFind(collaborationConfigName).(*collaborationConfig); ok {
		return config
	}
	log.Fatal().Msg("can't find collaboration config")
	return nil
}
