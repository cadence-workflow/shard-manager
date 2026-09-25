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
	"time"

	"github.com/uber-go/tally/m3"
	"github.com/uber-go/tally/prometheus"
	"gopkg.in/yaml.v2" // CAUTION: go.uber.org/config does not support yaml.v3

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	sdconfig "github.com/cadence-workflow/shard-manager/service/sharddistributor/config"
)

type (
	// Config contains the configuration for the shard-manager service.
	Config struct {
		RPC                  RPC                                 `yaml:"rpc"`
		Metrics              Metrics                             `yaml:"metrics"`
		PProf                PProf                               `yaml:"pprof"`
		Log                  Logger                              `yaml:"log"`
		ClusterGroupMetadata *ClusterGroupMetadata               `yaml:"clusterGroupMetadata"`
		DynamicConfigClient  dynamicconfig.FileBasedClientConfig `yaml:"dynamicConfigClient"`
		DynamicConfig        DynamicConfig                       `yaml:"dynamicconfig"`
		Authorization        Authorization                       `yaml:"authorization"`
		ShardDistribution    sdconfig.ShardDistribution          `yaml:"shardDistribution"`
		Histograms           metrics.HistogramMigration          `yaml:"histograms"`
		Gauges               metrics.GaugeMigration              `yaml:"gauge-migration"`
		Counters             metrics.CounterMigration            `yaml:"counter-migration"`
	}

	DynamicConfig struct {
		Client    string                              `yaml:"client"`
		FileBased dynamicconfig.FileBasedClientConfig `yaml:"filebased"`
	}

	// Service groups the configuration shared by service infrastructure.
	Service struct {
		RPC     RPC
		Metrics Metrics
		PProf   PProf
	}

	PProf struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	}

	RPC struct {
		GRPCPort        uint16 `yaml:"grpcPort"`
		BindOnLocalHost bool   `yaml:"bindOnLocalHost"`
		BindOnIP        string `yaml:"bindOnIP"`
		GRPCMaxMsgSize  int    `yaml:"grpcMaxMsgSize"`
		TLS             TLS    `yaml:"tls"`
	}

	Logger struct {
		Stdout     bool   `yaml:"stdout"`
		Level      string `yaml:"level"`
		OutputFile string `yaml:"outputFile"`
		LevelKey   string `yaml:"levelKey"`
		Encoding   string `yaml:"encoding"`
	}

	Metrics struct {
		M3                *m3.Configuration         `yaml:"m3"`
		Statsd            *Statsd                   `yaml:"statsd"`
		Prometheus        *prometheus.Configuration `yaml:"prometheus"`
		Tags              map[string]string         `yaml:"tags"`
		Prefix            string                    `yaml:"prefix"`
		ReportingInterval time.Duration             `yaml:"reportingInterval"`
	}

	Statsd struct {
		HostPort      string        `yaml:"hostPort" validate:"nonzero"`
		Prefix        string        `yaml:"prefix" validate:"nonzero"`
		FlushInterval time.Duration `yaml:"flushInterval"`
		FlushBytes    int           `yaml:"flushBytes"`
	}

	// YamlNode defers unmarshalling plugin-specific configuration.
	YamlNode struct {
		unmarshal func(out any) error
	}
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

// ToYamlNode converts configuration into a deferred YAML node.
func ToYamlNode(input any) (*YamlNode, error) {
	data, err := yaml.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("could not serialize data to yaml: %w", err)
	}
	var out *YamlNode
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("could not deserialize to yaml node: %w", err)
	}
	return out, nil
}

// ValidateAndFillDefaults validates this config and fills default values if needed.
func (c *Config) ValidateAndFillDefaults() error {
	c.fillDefaults()
	return c.validate()
}

func (c *Config) validate() error {
	if err := c.ClusterGroupMetadata.Validate(); err != nil {
		return err
	}
	return c.Authorization.Validate()
}

func (c *Config) fillDefaults() {}

// String converts the config object into a string.
func (c *Config) String() string {
	out, _ := json.MarshalIndent(c, "", "    ")
	return string(out)
}

// ServiceConfig returns service-level config from the top-level fields.
func (c *Config) ServiceConfig() Service {
	return Service{
		RPC:     c.RPC,
		Metrics: c.Metrics,
		PProf:   c.PProf,
	}
}
