package config

import "time"

// Config is the configuration for the shard distributor canary
type Config struct {
	Canary CanaryConfig `yaml:"canary"`
}

type CanaryConfig struct {
	// NumFixedExecutors is the number of executors of fixed namespace
	// Values more than 1 will create multiple executors processing the same fixed namespace
	// Default: 1
	NumFixedExecutors int `yaml:"numFixedExecutors"`

	// NumEphemeralExecutors is the number of executors of ephemeral namespace
	// Values more than 1 will create multiple executors processing the same ephemeral namespace
	// Default: 1
	NumEphemeralExecutors int `yaml:"numEphemeralExecutors"`

	// Load configures the periodic load run. The run is disabled when Load.Namespace is empty.
	Load LoadConfig `yaml:"load"`
}

// LoadConfig configures the periodic load run against a dedicated ephemeral namespace.
// A run only happens if the shard distributor also has Namespace configured.
type LoadConfig struct {
	// Namespace is the namespace the load run targets
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
}
