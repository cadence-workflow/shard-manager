package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	canaryConfig "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	cfg := canaryConfig.Config{
		Canary: canaryConfig.CanaryConfig{
			Fixed:     canaryConfig.NamespaceConfig{Name: defaultFixedNamespace, NumExecutors: defaultNumExecutors},
			Ephemeral: canaryConfig.NamespaceConfig{Name: defaultEphemeralNamespace, NumExecutors: defaultNumExecutors},
		},
	}

	assert.NoError(t, fx.ValidateApp(opts(defaultShardDistributorEndpoint, defaultCanaryGRPCPort, cfg)))
}
