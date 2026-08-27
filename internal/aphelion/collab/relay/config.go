package relay

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"sdmm/internal/aphelion/collab/protocolv2"
)

type Duration time.Duration

func (duration *Duration) UnmarshalYAML(node *yaml.Node) error {
	var encoded string
	if err := node.Decode(&encoded); err != nil {
		return fmt.Errorf("duration must use Go duration syntax: %w", err)
	}
	parsed, err := time.ParseDuration(encoded)
	if err != nil {
		return fmt.Errorf("parse duration: %w", err)
	}
	*duration = Duration(parsed)
	return nil
}

func (duration Duration) Time() time.Duration {
	return time.Duration(duration)
}

type Config struct {
	Version           uint                `yaml:"version"`
	PublicOrigin      string              `yaml:"public_origin"`
	BindAddress       string              `yaml:"bind_address"`
	TrustedProxyCIDRs []string            `yaml:"trusted_proxy_cidrs"`
	RoomIdleTTL       Duration            `yaml:"room_idle_ttl"`
	Limits            Limits              `yaml:"limits"`
	Observability     ObservabilityConfig `yaml:"observability"`
}

type Limits struct {
	MaxRooms              int      `yaml:"max_rooms"`
	MaxConnections        int      `yaml:"max_connections"`
	MaxConnectionsPerRoom int      `yaml:"max_connections_per_room"`
	MaxMessageBytes       int      `yaml:"max_message_bytes"`
	MaxRoomBytesPerSecond int64    `yaml:"max_room_bytes_per_second"`
	ConnectBurst          int      `yaml:"connect_burst"`
	ConnectWindow         Duration `yaml:"connect_window"`
	MessageBurst          int      `yaml:"message_burst"`
	MessageWindow         Duration `yaml:"message_window"`
}

type ObservabilityConfig struct {
	LogLevel           string `yaml:"log_level"`
	MetricsBindAddress string `yaml:"metrics_bind_address"`
}

func LoadConfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, fmt.Errorf("relay config path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open relay config: %w", err)
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode relay config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return Config{}, fmt.Errorf("relay config contains multiple YAML documents")
	} else if !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("decode trailing relay config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Version != 1 {
		return fmt.Errorf("relay config version %d is unsupported", config.Version)
	}
	origin, err := url.Parse(config.PublicOrigin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return fmt.Errorf("relay public_origin must be an HTTPS origin without credentials, path, query, or fragment")
	}
	if _, _, err := net.SplitHostPort(config.BindAddress); err != nil {
		return fmt.Errorf("relay bind_address is invalid: %w", err)
	}
	if config.RoomIdleTTL.Time() <= 0 {
		return fmt.Errorf("relay room_idle_ttl must be positive")
	}
	for _, encoded := range config.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(encoded); err != nil {
			return fmt.Errorf("trusted proxy CIDR is invalid")
		}
	}
	if err := config.Limits.validate(); err != nil {
		return err
	}
	switch config.Observability.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("observability log_level is invalid")
	}
	if _, _, err := net.SplitHostPort(config.Observability.MetricsBindAddress); err != nil {
		return fmt.Errorf("metrics_bind_address is invalid: %w", err)
	}
	return nil
}

func (limits Limits) validate() error {
	if limits.MaxRooms <= 0 || limits.MaxConnections <= 0 || limits.MaxConnectionsPerRoom <= 0 || limits.MaxConnectionsPerRoom > limits.MaxConnections ||
		limits.MaxMessageBytes <= 0 || limits.MaxMessageBytes > protocolv2.MaxCiphertextBytes || limits.MaxRoomBytesPerSecond <= 0 ||
		limits.ConnectBurst <= 0 || limits.ConnectWindow.Time() <= 0 || limits.MessageBurst <= 0 || limits.MessageWindow.Time() <= 0 {
		return fmt.Errorf("relay limits must be positive, bounded, and internally consistent")
	}
	return nil
}
