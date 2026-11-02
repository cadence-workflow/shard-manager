package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	canaryConfig "github.com/cadence-workflow/shard-manager/service/sharddistributor/canary/config"
)

func TestDependenciesAreSatisfied(t *testing.T) {
	tests := []struct {
		name          string
		loadNamespace string
	}{
		{name: "load run disabled", loadNamespace: defaultLoadNamespace},
		{name: "load run enabled", loadNamespace: "shard-distributor-canary-load"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := canaryConfig.Config{
				Canary: canaryConfig.CanaryConfig{
					Fixed:     canaryConfig.NamespaceConfig{Name: defaultFixedNamespace, NumExecutors: defaultNumExecutors},
					Ephemeral: canaryConfig.NamespaceConfig{Name: defaultEphemeralNamespace, NumExecutors: defaultNumExecutors},
					Load: canaryConfig.LoadConfig{
						Namespace:       tt.loadNamespace,
						Interval:        defaultLoadInterval,
						NumShards:       defaultLoadNumShards,
						ShardsPerSecond: defaultLoadShardsPerSecond,
						NumExecutors:    defaultLoadNumExecutors,
						Hold:            defaultLoadHold,
						MaxFailureRatio: defaultLoadMaxFailureRatio,
					},
				},
			}

			// Port 0 lets each case bind its own listener.
			assert.NoError(t, fx.ValidateApp(opts(defaultShardDistributorEndpoint, 0, cfg)))
		})
	}
}
