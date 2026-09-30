package config

import (
	"testing"
	"time"

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
	assert.NotNil(t, config.EphemeralAssignmentCoalescingWindow)
	assert.Equal(t, 10*time.Millisecond, config.EphemeralAssignmentCoalescingWindow("test-namespace"))
	assert.NotNil(t, config.LoadBalancingNaive.MaxDeviation)
	assert.NotNil(t, config.LoadBalancingGreedy.PerShardCooldown)
	assert.NotNil(t, config.LoadBalancingGreedy.LoadSmoothingTimeConstant)
	assert.NotNil(t, config.LoadBalancingGreedy.MoveBudgetProportion)
	assert.NotNil(t, config.LoadBalancingGreedy.HysteresisUpperBand)
	assert.NotNil(t, config.LoadBalancingGreedy.HysteresisLowerBand)
	assert.NotNil(t, config.LoadBalancingGreedy.SevereImbalanceRatio)
}

func TestEphemeralAssignmentCoalescingWindowUpdates(t *testing.T) {
	client := dynamicconfig.NewInMemoryClient()
	dc := dynamicconfig.NewCollection(client, testlogger.New(t))
	config := NewConfig(dc)

	assert.Equal(t, 10*time.Millisecond, config.EphemeralAssignmentCoalescingWindow("test-namespace"))
	require.NoError(t, client.UpdateValue(dynamicproperties.ShardDistributorEphemeralAssignmentCoalescingWindow, 100*time.Millisecond))
	assert.Equal(t, 100*time.Millisecond, config.EphemeralAssignmentCoalescingWindow("test-namespace"))
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
