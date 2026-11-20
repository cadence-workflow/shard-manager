package authorization

import "context"

type nopAuthorizer struct{}

// NewNopAuthorizer returns an Authorizer that allows every request.
// It is the default when no Authorizer is wired into the fx container.
func NewNopAuthorizer() Authorizer {
	return &nopAuthorizer{}
}

func (a *nopAuthorizer) Authorize(_ context.Context, _ *Attributes) (Result, error) {
	return Result{Decision: DecisionAllow}, nil
}
