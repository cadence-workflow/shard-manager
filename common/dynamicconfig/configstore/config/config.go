package config

import "time"

// This package is necessary to avoid import cycle as config_store_client imports common/config
// while common/config imports this ClientConfig definition

// ClientConfig is the config for the config store based dynamic config client.
// It specifies how often the cached config should be updated by checking underlying database.
type ClientConfig struct {
	PollInterval        time.Duration `yaml:"pollInterval"`
	UpdateRetryAttempts int           `yaml:"updateRetryAttempts"`
	FetchTimeout        time.Duration `yaml:"FetchTimeout"`
	UpdateTimeout       time.Duration `yaml:"UpdateTimeout"`
}
