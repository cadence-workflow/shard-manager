package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	canaryConfig "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	assert.NoError(t, fx.ValidateApp(opts(
		defaultFixedNamespace,
		defaultEphemeralNamespace,
		defaultShardDistributorEndpoint,
		defaultCanaryGRPCPort,
		defaultNumExecutors,
		defaultNumExecutors,
		canaryConfig.LoadConfig{
			Namespace:       defaultLoadNamespace,
			Interval:        defaultLoadInterval,
			NumShards:       defaultLoadNumShards,
			ShardsPerSecond: defaultLoadShardsPerSecond,
			NumExecutors:    defaultLoadNumExecutors,
			Hold:            defaultLoadHold,
		},
	)))
}
