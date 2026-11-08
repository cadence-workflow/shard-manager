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
func (c CanaryConfig) ClientConfig() clientcommon.Config {
	namespaces := []clientcommon.NamespaceConfig{
		{Namespace: c.Fixed.Name, HeartBeatInterval: heartBeatInterval},
		{Namespace: c.Ephemeral.Name, HeartBeatInterval: heartBeatInterval},
	}
	if c.Load.Namespace != "" {
		namespaces = append(namespaces, clientcommon.NamespaceConfig{Namespace: c.Load.Namespace, HeartBeatInterval: heartBeatInterval})
	}
	return clientcommon.Config{Namespaces: namespaces}
}

// LoadConfig configures the periodic load run against a dedicated fixed namespace.
type LoadConfig struct {
	// Namespace is the namespace the load run targets
	// Empty disables the load run.
	Namespace string `yaml:"namespace"`

	// NumShards is the number of shards for the namespace
	NumShards int `yaml:"numShards"`

	// Interval is the time between runs, before jitter
	Interval time.Duration `yaml:"interval"`

	// BaselineExecutors is the number of executors each canary instance runs constantly
	BaselineExecutors int `yaml:"baselineExecutors"`

	// NumExecutors is the number of executors each canary instance adds on top
	// of its baseline during a run
	NumExecutors int `yaml:"numExecutors"`

	// ExecutorsPerSecond is the rate at which a run starts and stops the extra executors
	ExecutorsPerSecond int `yaml:"executorsPerSecond"`

	// Hold is how long a run keeps extra executors running before stopping them
	Hold time.Duration `yaml:"hold"`

	// ConvergenceTimeout is how long a run waits, after scaling up and after
	// scaling down, for GetShardOwner to agree with the local shard holders
	ConvergenceTimeout time.Duration `yaml:"convergenceTimeout"`
}
