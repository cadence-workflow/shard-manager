package config

import (
	"time"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/clientcommon"
)

const heartBeatInterval = time.Second

// Config is the configuration for the shard distributor canary
type Config struct {
	Canary CanaryConfig `yaml:"canary"`
}

type CanaryConfig struct {
	// Fixed is the namespace with a fixed set of shards
	Fixed NamespaceConfig `yaml:"fixed"`

	// Ephemeral is the namespace the canary creates shards in
	Ephemeral NamespaceConfig `yaml:"ephemeral"`
}

// NamespaceConfig configures the executors the canary runs for one namespace.
type NamespaceConfig struct {
	// Name is the namespace name
	Name string `yaml:"name"`

	// NumExecutors is the number of executors processing the namespace
	// Values more than 1 will create multiple executors processing the same namespace
	// Default: 1
	NumExecutors int `yaml:"numExecutors"`
}

// ClientConfig returns the executor and spectator client config for the canary namespaces.
func (c CanaryConfig) ClientConfig() clientcommon.Config {
	return clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{Namespace: c.Fixed.Name, HeartBeatInterval: heartBeatInterval},
			{Namespace: c.Ephemeral.Name, HeartBeatInterval: heartBeatInterval},
		},
	}
}
