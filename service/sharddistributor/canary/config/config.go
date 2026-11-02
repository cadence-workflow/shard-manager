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

	// Load configures the periodic load run. The run is disabled when Load.Namespace is empty.
	Load LoadConfig `yaml:"load"`
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
// The load namespace is not included: load executors are built per run with their own config.
func (c CanaryConfig) ClientConfig() clientcommon.Config {
	return clientcommon.Config{
		Namespaces: []clientcommon.NamespaceConfig{
			{Namespace: c.Fixed.Name, HeartBeatInterval: heartBeatInterval},
			{Namespace: c.Ephemeral.Name, HeartBeatInterval: heartBeatInterval},
		},
	}
}

// LoadConfig configures the periodic load run against a dedicated ephemeral namespace.
type LoadConfig struct {
	// Namespace is the namespace the load run targets. The shard distributor must
	// have it configured as an ephemeral namespace. Empty disables the load run.
	Namespace string `yaml:"namespace"`

	// Interval is the time between runs, before jitter
	Interval time.Duration `yaml:"interval"`

	// NumShards is the number of shard keys each run creates
	NumShards int `yaml:"numShards"`

	// ShardsPerSecond is the rate at which a run creates shard keys
	ShardsPerSecond int `yaml:"shardsPerSecond"`

	// NumExecutors is the number of extra executors each run starts
	NumExecutors int `yaml:"numExecutors"`

	// Hold is how long a run keeps its peak before releasing the shards
	Hold time.Duration `yaml:"hold"`

	// MaxFailureRatio is the fraction of shard creations, in [0, 1], that may
	// fail before the run is reported as failed
	MaxFailureRatio float64 `yaml:"maxFailureRatio"`
}
