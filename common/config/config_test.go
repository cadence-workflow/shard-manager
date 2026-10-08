// Copyright (c) 2019 Uber Technologies, Inc.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	var cfg Config
	require.NoError(t, Load("", "../../config", "", &cfg))
	require.NoError(t, cfg.ValidateAndFillDefaults())
	assert.NotEmpty(t, cfg.String())
}

func TestConfigErrorInAuthorizationConfig(t *testing.T) {
	cfg := &Config{
		Authorization: Authorization{
			OAuthAuthorizer: OAuthAuthorizer{Enable: true},
			NoopAuthorizer:  NoopAuthorizer{Enable: true},
		},
		ClusterGroupMetadata: &ClusterGroupMetadata{CurrentClusterName: "test"},
	}

	require.Error(t, cfg.ValidateAndFillDefaults())
}

func TestServiceConfig(t *testing.T) {
	cfg := Config{
		RPC:     RPC{GRPCPort: 123},
		Metrics: Metrics{Prefix: "test"},
		PProf:   PProf{Port: 456},
	}

	svc := cfg.ServiceConfig()
	assert.Equal(t, uint16(123), svc.RPC.GRPCPort)
	assert.Equal(t, "test", svc.Metrics.Prefix)
	assert.Equal(t, 456, svc.PProf.Port)
}

func TestYamlNode(t *testing.T) {
	type pluginConfig struct {
		Endpoint string `yaml:"endpoint"`
	}
	node, err := ToYamlNode(pluginConfig{Endpoint: "localhost:2379"})
	require.NoError(t, err)

	var decoded pluginConfig
	require.NoError(t, node.Decode(&decoded))
	assert.Equal(t, "localhost:2379", decoded.Endpoint)
}
