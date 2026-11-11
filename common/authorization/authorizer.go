//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination authorizer_mock.go -self_package github.com/cadence-workflow/shard-manager/common/authorization

// Package authorization defines the Authorizer interface used by shard-distributor
// RPC wrappers to enforce per-API permissions. The default implementation is a
// no-op that allows every request; deployments can inject an alternative
// implementation via fx.
package authorization

import "context"

const (
	// DecisionDeny means auth decision is denied.
	DecisionDeny Decision = iota + 1
	// DecisionAllow means auth decision is allowed.
	DecisionAllow
)

const (
	// PermissionRead means the caller can invoke read-only namespace APIs.
	PermissionRead Permission = iota + 1
	// PermissionWrite means the caller can invoke namespace mutation APIs.
	PermissionWrite
	// PermissionAdmin means the caller can invoke admin-only APIs.
	PermissionAdmin
)

type (
	// Decision is the enum type for an auth decision.
	Decision int

	// Permission is the enum type for the permission required by an API.
	Permission int

	// Attributes is the input the Authorizer makes its decision on.
	// Fields are intentionally minimal; extend (e.g., with Actor, RequestBody)
	// when a non-nop authorizer needs them.
	Attributes struct {
		APIName    string
		Namespace  string
		Permission Permission
	}

	// Result is the output of an authorization decision.
	Result struct {
		Decision Decision
	}

	// Authorizer evaluates whether a request described by Attributes is allowed.
	Authorizer interface {
		Authorize(ctx context.Context, attributes *Attributes) (Result, error)
	}
)
