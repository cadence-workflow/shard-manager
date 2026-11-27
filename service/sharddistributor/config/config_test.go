package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cadence-workflow/shard-manager/common/dynamicconfig"
	"github.com/cadence-workflow/shard-manager/common/dynamicconfig/dynamicproperties"
	"github.com/cadence-workflow/shard-manager/common/log/testlogger"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestNewDynamicConfigCreatesInstanceWithProperties(t *testing.T) {
	dc := dynamicconfig.NewNopCollection()

	config := NewConfig(dc)

	assert.NotNil(t, config)
	assert.NotNil(t, config.LoadBalancingMode)
	assert.Equal(t, 0, config.FixedShardNum("test-namespace"))
	assert.NotNil(t, config.LoadBalancingNaive.MaxDeviation)
	assert.NotNil(t, config.LoadBalancingGreedy.PerShardCooldown)
	assert.NotNil(t, config.LoadBalancingGreedy.LoadSmoothingTimeConstant)
	assert.NotNil(t, config.LoadBalancingGreedy.MoveBudgetProportion)
	assert.NotNil(t, config.LoadBalancingGreedy.HysteresisUpperBand)
	assert.NotNil(t, config.LoadBalancingGreedy.HysteresisLowerBand)
	assert.NotNil(t, config.LoadBalancingGreedy.SevereImbalanceRatio)
}

func TestGetLoadBalancingMode(t *testing.T) {
	tests := []struct {
		name         string
		configValue  string
		expectedMode types.LoadBalancingMode
	}{
		{
			name:         "Naive",
			configValue:  "naive",
			expectedMode: types.LoadBalancingModeNAIVE,
		},
		{
			name:         "Greedy",
			configValue:  "greedy",
			expectedMode: types.LoadBalancingModeGREEDY,
		},
		{
			name:         "Invalid",
			configValue:  "invalid",
			expectedMode: types.LoadBalancingModeINVALID,
		},
		{
			name:         "Empty",
			configValue:  "",
			expectedMode: types.LoadBalancingModeINVALID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := dynamicconfig.NewInMemoryClient()
			err := client.UpdateValue(dynamicproperties.ShardDistributorLoadBalancingMode, tt.configValue)
			require.NoError(t, err)
			dc := dynamicconfig.NewCollection(client, testlogger.New(t))
			config := NewConfig(dc)

			mode := config.GetLoadBalancingMode("test-namespace")
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestEffectiveNamespace(t *testing.T) {
	namespace := Namespace{Name: "test-namespace", Type: NamespaceTypeFixed, ShardNum: 32}

	tests := []struct {
		name          string
		static        Namespace
		fixedShardNum int
		want          Namespace
	}{
		{
			name:          "zero keeps the static config",
			static:        namespace,
			fixedShardNum: 0,
			want:          namespace,
		},
		{
			name:          "negative keeps the static config",
			static:        namespace,
			fixedShardNum: -1,
			want:          namespace,
		},
		{
			name:          "positive replaces the shard count",
			static:        namespace,
			fixedShardNum: 64,
			want:          Namespace{Name: "test-namespace", Type: NamespaceTypeFixed, ShardNum: 64},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveNamespace(tt.static, tt.fixedShardNum)
			assert.Equal(t, tt.want, got)
		})
	}
}
