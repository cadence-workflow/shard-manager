package authorization

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNopAuthorizer_AllowsEveryRequest(t *testing.T) {
	tests := []struct {
		name  string
		attrs *Attributes
	}{
		{
			name:  "nil attributes",
			attrs: nil,
		},
		{
			name:  "empty attributes",
			attrs: &Attributes{},
		},
		{
			name: "read on namespace",
			attrs: &Attributes{
				APIName:    "GetNamespaceState",
				Namespace:  "ns1",
				Permission: PermissionRead,
			},
		},
		{
			name: "admin without namespace",
			attrs: &Attributes{
				APIName:    "AdminAPI",
				Permission: PermissionAdmin,
			},
		},
	}

	authz := NewNopAuthorizer()
	require.NotNil(t, authz)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := authz.Authorize(context.Background(), tc.attrs)
			require.NoError(t, err)
			assert.Equal(t, DecisionAllow, result.Decision)
		})
	}
}

func TestPermission_DistinctValues(t *testing.T) {
	assert.NotEqual(t, PermissionRead, PermissionWrite)
	assert.NotEqual(t, PermissionRead, PermissionAdmin)
	assert.NotEqual(t, PermissionWrite, PermissionAdmin)
}

func TestDecision_DistinctValues(t *testing.T) {
	assert.NotEqual(t, DecisionAllow, DecisionDeny)
}
