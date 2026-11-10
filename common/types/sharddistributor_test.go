package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetShardOwnerRequest_GetShardKey(t *testing.T) {
	tests := []struct {
		name string
		req  *GetShardOwnerRequest
		want string
	}{
		{
			name: "nil request",
			req:  nil,
			want: "",
		},
		{
			name: "empty request",
			req:  &GetShardOwnerRequest{},
			want: "",
		},
		{
			name: "has shard key",
			req:  &GetShardOwnerRequest{ShardKey: "shard-key"},
			want: "shard-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.GetShardKey()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetShardOwnerRequest_GetNamespace(t *testing.T) {
	tests := []struct {
		name string
		req  *GetShardOwnerRequest
		want string
	}{
		{
			name: "nil request",
			req:  nil,
			want: "",
		},
		{
			name: "empty request",
			req:  &GetShardOwnerRequest{},
			want: "",
		},
		{
			name: "has namespace",
			req:  &GetShardOwnerRequest{Namespace: "namespace"},
			want: "namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.GetNamespace()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetShardOwnerResponse_GetOwner(t *testing.T) {
	tests := []struct {
		name string
		resp *GetShardOwnerResponse
		want string
	}{
		{
			name: "nil response",
			resp: nil,
			want: "",
		},
		{
			name: "empty response",
			resp: &GetShardOwnerResponse{},
			want: "",
		},
		{
			name: "has owner",
			resp: &GetShardOwnerResponse{Owner: "owner"},
			want: "owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resp.GetOwner()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetShardOwnerResponse_GetNamespace(t *testing.T) {
	tests := []struct {
		name string
		resp *GetShardOwnerResponse
		want string
	}{
		{
			name: "nil response",
			resp: nil,
			want: "",
		},
		{
			name: "empty response",
			resp: &GetShardOwnerResponse{},
			want: "",
		},
		{
			name: "has namespace",
			resp: &GetShardOwnerResponse{Namespace: "namespace"},
			want: "namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.resp.GetNamespace()
			assert.Equal(t, tt.want, got)
		})
	}
}
