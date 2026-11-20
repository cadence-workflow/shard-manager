package timeout

import (
	"context"
	"time"
)

const (
	// AdminDefaultTimeout is the admin service default timeout used to make calls
	AdminDefaultTimeout = 10 * time.Second
	// AdminDefaultLargeTimeout is the admin service large default timeout used to make calls
	AdminDefaultLargeTimeout = time.Minute
	// FrontendDefaultTimeout is the frontend service default timeout used to make calls
	FrontendDefaultTimeout = 10 * time.Second
	// FrontendDefaultLongPollTimeout is the frontend service long poll default timeout used to make calls
	FrontendDefaultLongPollTimeout = time.Minute * 3
	// MatchingDefaultTimeout is the default timeout used to make calls
	MatchingDefaultTimeout = time.Minute
	// MatchingDefaultLongPollTimeout is the long poll default timeout used to make calls
	MatchingDefaultLongPollTimeout = time.Minute * 2
	// HistoryDefaultTimeout is the default timeout used to make calls
	HistoryDefaultTimeout = time.Second * 30
	// ShardDistributorDefaultTimeout is the default timeout used to make calls
	ShardDistributorDefaultTimeout = time.Second * 10
	// ShardDistributorExecutorDefaultTimeout is the default timeout used to make calls
	ShardDistributorExecutorDefaultTimeout = time.Second * 10
)

func createContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout > 0 {
		return context.WithTimeout(parent, timeout)
	}
	return context.WithCancel(parent)
}
