// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/uber-go/tally/m3"
	"github.com/uber-go/tally/prometheus"
	yarpctls "go.uber.org/yarpc/api/transport/tls"
	"gopkg.in/yaml.v2" // CAUTION: go.uber.org/config does not support yaml.v3

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	c "github.com/cadence-workflow/shard-manager/common/dynamicconfig/configstore/config"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/service"
)

type (
	// Config contains the configuration for a set of cadence services
	Config struct {
		// Log is the logging config
		Log Logger `yaml:"log"`
		// Services is a map of service name to service config items
		Services map[string]Service `yaml:"services"`
		// DynamicConfigClient is the config for setting up the file based dynamic config client
		// Filepath would be relative to the root directory when the path wasn't absolute.
		// Included for backwards compatibility, please transition to DynamicConfig
		// If both are specified, DynamicConig will be used.
		DynamicConfigClient dynamicconfig.FileBasedClientConfig `yaml:"dynamicConfigClient"`
		// DynamicConfig is the config for setting up all dynamic config clients
		// Allows for changes in client without needing code change
		DynamicConfig DynamicConfig `yaml:"dynamicconfig"`
		// Authorization is the config for setting up authorization
		Authorization Authorization `yaml:"authorization"`
		// HeaderForwardingRules defines which inbound headers to include or exclude on outbound calls
		HeaderForwardingRules []HeaderRule `yaml:"headerForwardingRules"`
		// ShardDistributorClient is the config for shard distributor client
		// Shard distributor is used to distribute shards across multiple cadence service instances
		// Note: This is not recommended for use, it's still experimental
		ShardDistributorClient ShardDistributorClient `yaml:"shardDistributorClient"`

		// ShardDistribution is a config for the shard distributor leader election component that allows to run a single process per region and manage shard namespaces.
		ShardDistribution ShardDistribution `yaml:"shardDistribution"`

		// Histograms controls timer vs histogram metric emission while they are being migrated.
		//
		// Timers will eventually be dropped, and this config will be validation-only (e.g. to error if any explicitly request timers).
		Histograms metrics.HistogramMigration `yaml:"histograms"`
	}

	// Membership holds peer provider configuration.
	Membership struct {
		Provider PeerProvider `yaml:"provider"`
	}

	// PeerProvider is provider config. Contents depends on plugin in use
	PeerProvider map[string]*YamlNode

	HeaderRule struct {
		Add   bool // if false, matching headers are removed if previously matched.
		Match *regexp.Regexp
	}

	DynamicConfig struct {
		Client      string                              `yaml:"client"`
		ConfigStore c.ClientConfig                      `yaml:"configstore"`
		FileBased   dynamicconfig.FileBasedClientConfig `yaml:"filebased"`
	}

	// Service contains the service specific config items
	Service struct {
		// TChannel is the tchannel configuration
		RPC RPC `yaml:"rpc"`
		// Metrics is the metrics subsystem configuration
		Metrics Metrics `yaml:"metrics"`
		// PProf is the PProf configuration
		PProf PProf `yaml:"pprof"`
	}

	// PProf contains the rpc config items
	PProf struct {
		// Port is the port on which the PProf will bind to
		Port int `yaml:"port"`
		// Host is the host on which the PProf will bind to, default to `localhost`
		Host string `yaml:"host"`
	}

	// RPC contains the rpc config items
	RPC struct {
		// Port is the port  on which the Thrift TChannel will bind to
		Port uint16 `yaml:"port"`
		// GRPCPort is the port on which the grpc listener will bind to
		GRPCPort uint16 `yaml:"grpcPort"`
		// BindOnLocalHost is true if localhost is the bind address
		BindOnLocalHost bool `yaml:"bindOnLocalHost"`
		// BindOnIP can be used to bind service on specific ip (eg. `0.0.0.0`) -
		// check net.ParseIP for supported syntax, only IPv4 is supported,
		// mutually exclusive with `BindOnLocalHost` option
		BindOnIP string `yaml:"bindOnIP"`
		// DisableLogging disables all logging for rpc
		DisableLogging bool `yaml:"disableLogging"`
		// LogLevel is the desired log level
		LogLevel string `yaml:"logLevel"`
		// GRPCMaxMsgSize allows overriding default (4MB) message size for gRPC
		GRPCMaxMsgSize int `yaml:"grpcMaxMsgSize"`
		// TLS allows configuring optional TLS/SSL authentication on the server (only on gRPC port)
		TLS TLS `yaml:"tls"`
		// HTTP keeps configuration for exposed HTTP API
		HTTP *HTTP `yaml:"http"`
	}

	// HTTP API configuration
	HTTP struct {
		// Port for listening HTTP requests
		Port uint16 `yaml:"port"`
		// List of RPC procedures available to call using HTTP
		Procedures []string `yaml:"procedures"`
		// TLS allows configuring TLS/SSL for HTTP requests
		TLS TLS `yaml:"tls"`
		// Mode represents the TLS mode of the transport.
		// Available modes: disabled, permissive, enforced
		TLSMode yarpctls.Mode `yaml:"TLSMode"`
	}

	// Logger contains the config items for logger
	Logger struct {
		// Stdout is true then the output needs to goto standard out
		// By default this is false and output will go to standard error
		Stdout bool `yaml:"stdout"`
		// Level is the desired log level
		Level string `yaml:"level"`
		// OutputFile is the path to the log output file
		// Stdout must be false, otherwise Stdout will take precedence
		OutputFile string `yaml:"outputFile"`
		// LevelKey is the desired log level, defaults to "level"
		LevelKey string `yaml:"levelKey"`
		// Encoding decides the format, supports "console" and "json".
		// "json" will print the log in JSON format(better for machine), while "console" will print in plain-text format(more human friendly)
		// Default is "json"
		Encoding string `yaml:"encoding"`
	}

	// Metrics contains the config items for metrics subsystem
	Metrics struct {
		// M3 is the configuration for m3 metrics reporter
		M3 *m3.Configuration `yaml:"m3"`
		// Statsd is the configuration for statsd reporter
		Statsd *Statsd `yaml:"statsd"`
		// Prometheus is the configuration for prometheus reporter
		// Some documentation below because the tally library is missing it:
		// In this configuration, default timerType is "histogram", alternatively "summary" is also supported.
		// In some cases, summary is better. Choose it wisely.
		// For histogram, default buckets are defined in https://github.com/cadence-workflow/shard-manager/blob/master/common/metrics/tally/prometheus/buckets.go#L34
		// For summary, default objectives are defined in https://github.com/uber-go/tally/blob/137973e539cd3589f904c23d0b3a28c579fd0ae4/prometheus/reporter.go#L70
		// You can customize the buckets/objectives if the default is not good enough.
		Prometheus *prometheus.Configuration `yaml:"prometheus"`
		// Tags is the set of key-value pairs to be reported
		// as part of every metric
		Tags map[string]string `yaml:"tags"`
		// Prefix sets the prefix to all outgoing metrics
		Prefix string `yaml:"prefix"`
		// ReportingInterval is the interval of metrics reporter
		ReportingInterval time.Duration `yaml:"reportingInterval"` // defaults to 1s
	}

	// Statsd contains the config items for statsd metrics reporter
	Statsd struct {
		// The host and port of the statsd server
		HostPort string `yaml:"hostPort" validate:"nonzero"`
		// The prefix to use in reporting to statsd
		Prefix string `yaml:"prefix" validate:"nonzero"`
		// FlushInterval is the maximum interval for sending packets.
		// If it is not specified, it defaults to 1 second.
		FlushInterval time.Duration `yaml:"flushInterval"`
		// FlushBytes specifies the maximum udp packet size you wish to send.
		// If FlushBytes is unspecified, it defaults  to 1432 bytes, which is
		// considered safe for local traffic.
		FlushBytes int `yaml:"flushBytes"`
	}

	// ShardDistributorClient contains the config items for shard distributor
	ShardDistributorClient struct {
		// The host and port of the shard distributor server
		HostPort string `yaml:"hostPort"`
	}

	// YamlNode is a lazy-unmarshaler, because *yaml.Node only exists in gopkg.in/yaml.v3, not v2,
	// and go.uber.org/config currently uses only v2.
	YamlNode struct {
		unmarshal func(out any) error
	}

	// ShardDistribution is a configuration for leader election running.
	// This configuration should be in sync with sharddistributor.
	ShardDistribution struct {
		LeaderStore Store         `yaml:"leaderStore"`
		Election    Election      `yaml:"election"`
		Namespaces  []Namespace   `yaml:"namespaces"`
		Process     LeaderProcess `yaml:"process"`
		Store       Store         `yaml:"store"`
	}

	// Store is a generic container for any storage configuration that should be parsed by the implementation.
	Store struct {
		StorageParams *YamlNode `yaml:"storageParams"`
	}

	Namespace struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
		Mode string `yaml:"mode"`
		// ShardNum is defined for fixed namespace.
		ShardNum int64 `yaml:"shardNum"`
	}

	Election struct {
		LeaderPeriod           time.Duration `yaml:"leaderPeriod"`           // Time to hold leadership before resigning
		MaxRandomDelay         time.Duration `yaml:"maxRandomDelay"`         // Maximum random delay before campaigning
		FailedElectionCooldown time.Duration `yaml:"failedElectionCooldown"` // wait between election attempts with unhandled errors
	}

	LeaderProcess struct {
		Period       time.Duration `yaml:"period"`
		HeartbeatTTL time.Duration `yaml:"heartbeatTTL"`
	}
)

const (
	// NonShardedStoreName is the shard name used for singular (non-sharded) stores
	NonShardedStoreName = "NonShardedStore"

	FilestoreConfig = "filestore"
	S3storeConfig   = "s3store"
)

var _ yaml.Unmarshaler = (*YamlNode)(nil)

func (y *YamlNode) UnmarshalYAML(unmarshal func(interface{}) error) error {
	y.unmarshal = unmarshal
	return nil
}

func (y *YamlNode) Decode(out any) error {
	if y == nil {
		return nil
	}
	return y.unmarshal(out)
}

// ToYamlNode is a bit of a hack to get a *yaml.Node for config-parsing compatibility purposes.
// There is probably a better way to achieve this with yaml-loading compatibility, but this is at least fairly simple.
func ToYamlNode(input any) (*YamlNode, error) {
	data, err := yaml.Marshal(input)
	if err != nil {
		// should be extremely unlikely, unless yaml marshaling is customized
		return nil, fmt.Errorf("could not serialize data to yaml: %w", err)
	}
	var out *YamlNode
	err = yaml.Unmarshal(data, &out)
	if err != nil {
		// should not be possible
		return nil, fmt.Errorf("could not deserialize to yaml node: %w", err)
	}
	return out, nil
}

// String converts the config object into a string
func (c *Config) String() string {
	out, _ := json.MarshalIndent(c, "", "    ")
	return string(out)
}

func (c *Config) GetServiceConfig(serviceName string) (Service, error) {
	shortName := service.ShortName(serviceName)
	serviceConfig, ok := c.Services[shortName]
	if !ok {
		return Service{}, fmt.Errorf("no config section for service: %s", shortName)
	}
	return serviceConfig, nil
}
