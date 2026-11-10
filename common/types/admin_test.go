package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsolationGroupConfiguration_ToPartitionList(t *testing.T) {
	tests := map[string]struct {
		in          *IsolationGroupConfiguration
		expectedOut []IsolationGroupPartition
	}{
		"valid value": {
			in: &IsolationGroupConfiguration{
				"zone-1": {
					Name:  "zone-1",
					State: IsolationGroupStateDrained,
				},
				"zone-2": {
					Name:  "zone-2",
					State: IsolationGroupStateHealthy,
				},
				"zone-3": {
					Name:  "zone-3",
					State: IsolationGroupStateDrained,
				},
			},
			expectedOut: []IsolationGroupPartition{
				{
					Name:  "zone-1",
					State: IsolationGroupStateDrained,
				},
				{
					Name:  "zone-2",
					State: IsolationGroupStateHealthy,
				},
				{
					Name:  "zone-3",
					State: IsolationGroupStateDrained,
				},
			},
		},
	}
	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, td.expectedOut, td.in.ToPartitionList())
		})
	}
}

func TestIsolationGroupConfigurationDeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input IsolationGroupConfiguration
	}{
		{
			name:  "nil",
			input: nil,
		},
		{
			name:  "empty",
			input: IsolationGroupConfiguration{},
		},
		{
			name: "single group",
			input: IsolationGroupConfiguration{
				"zone-1": {
					Name:  "zone-1",
					State: IsolationGroupStateDrained,
				},
			},
		},
		{
			name: "multiple groups",
			input: IsolationGroupConfiguration{
				"zone-1": {
					Name:  "zone-1",
					State: IsolationGroupStateDrained,
				},
				"zone-2": {
					Name:  "zone-2",
					State: IsolationGroupStateHealthy,
				},
				"zone-3": {
					Name:  "zone-3",
					State: IsolationGroupStateDrained,
				},
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.DeepCopy()
			assert.Equal(t, tc.input, got)

			if tc.input == nil {
				return
			}

			tc.input["new"] = IsolationGroupPartition{Name: "new", State: IsolationGroupStateHealthy}
			assert.NotEqual(t, tc.input, got)
		})
	}
}
