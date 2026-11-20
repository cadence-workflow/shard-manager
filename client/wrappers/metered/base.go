package metered

import "context"

type retryCountKeyType string

const retryCountKey = retryCountKeyType("retryCount")

func getRetryCountFromContext(ctx context.Context) int {
	if retryCount, ok := ctx.Value(retryCountKey).(int); ok {
		return retryCount
	}
	return -1
}
