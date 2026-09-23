// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package common

import (
	"errors"
	"time"

	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	"github.com/cadence-workflow/shard-manager/common/types"
)

const (
	shardDistributorServiceOperationInitialInterval    = 200 * time.Millisecond
	shardDistributorServiceOperationMaxInterval        = 10 * time.Second
	shardDistributorServiceOperationExpirationInterval = 15 * time.Second
)

func CreateShardDistributorServiceRetryPolicy() backoff.RetryPolicy {
	policy := backoff.NewExponentialRetryPolicy(shardDistributorServiceOperationInitialInterval)
	policy.SetMaximumInterval(shardDistributorServiceOperationMaxInterval)
	policy.SetExpirationInterval(shardDistributorServiceOperationExpirationInterval)

	return policy
}

// IsServiceTransientError checks if the error is a transient error.
func IsServiceTransientError(err error) bool {

	var (
		typesInternalServiceError *types.InternalServiceError
		typesServiceBusyError     *types.ServiceBusyError
		yarpcErrorsStatus         *yarpcerrors.Status
	)

	switch {
	case errors.As(err, &typesInternalServiceError):
		return true
	case errors.As(err, &typesServiceBusyError):
		return true
	case errors.As(err, &yarpcErrorsStatus):
		// We only selectively retry the following yarpc errors client can safe retry with a backoff
		if yarpcerrors.IsUnavailable(err) ||
			yarpcerrors.IsUnknown(err) ||
			yarpcerrors.IsCancelled(err) ||
			yarpcerrors.IsInternal(err) {
			return true
		}
		return false
	}

	return false
}

// IsServiceBusyError checks if the error is a service busy error.
func IsServiceBusyError(err error) bool {
	switch err.(type) {
	case *types.ServiceBusyError:
		return true
	}
	return false
}

// DurationToDays converts time.Duration to number of 24 hour days
func DurationToDays(d time.Duration) int32 {
	return int32(d / (24 * time.Hour))
}

// DurationToSeconds converts time.Duration to number of seconds
func DurationToSeconds(d time.Duration) int64 {
	return int64(d / time.Second)
}

// DaysToDuration converts number of 24 hour days to time.Duration
func DaysToDuration(d int32) time.Duration {
	return time.Duration(d) * (24 * time.Hour)
}

// SecondsToDuration converts number of seconds to time.Duration
func SecondsToDuration(d int64) time.Duration {
	return time.Duration(d) * time.Second
}
