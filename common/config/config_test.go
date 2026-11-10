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
