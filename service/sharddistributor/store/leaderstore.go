package store

import (
	"context"
)

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination=leaderstore_mock.go Elector,Election

// Elector is an interface that provides a way to establish a session for election.
// It establishes connection and a session and provides Election to run for leader.
type Elector interface {
	CreateElection(ctx context.Context, namespace string) (Election, error)
}

// Election is an interface that establishes leader campaign.
type Election interface {
	// Campaign blocks until leadership is acquired. The returned context is cancelled when the term ends.
	Campaign(ctx context.Context, hostname string) (context.Context, error)
	// Resign releases leadership early so another instance can take over.
	Resign(ctx context.Context) error
	// Guard returns a transaction guard representing the current leadership term.
	// This guard can be passed to the generic store to perform leader-protected writes.
	Guard() GuardFunc
}
