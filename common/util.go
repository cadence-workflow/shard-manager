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
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/shard-manager/common/backoff"
	cadence_errors "github.com/cadence-workflow/shard-manager/common/errors"
	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
)

const (
	golandMapReserverNumberOfBytes = 48

	shardDistributorServiceOperationInitialInterval    = 200 * time.Millisecond
	shardDistributorServiceOperationMaxInterval        = 10 * time.Second
	shardDistributorServiceOperationExpirationInterval = 15 * time.Second

	contextExpireThreshold = 10 * time.Millisecond
)

// AwaitWaitGroup calls Wait on the given wait
// Returns true if the Wait() call succeeded before the timeout
// Returns false if the Wait() did not return before the timeout
func AwaitWaitGroup(wg *sync.WaitGroup, timeout time.Duration) bool {
	doneC := make(chan struct{})

	go func() {
		wg.Wait()
		close(doneC)
	}()

	select {
	case <-doneC:
		return true
	case <-time.After(timeout):
		return false
	}
}

func CreateShardDistributorServiceRetryPolicy() backoff.RetryPolicy {
	policy := backoff.NewExponentialRetryPolicy(shardDistributorServiceOperationInitialInterval)
	policy.SetMaximumInterval(shardDistributorServiceOperationMaxInterval)
	policy.SetExpirationInterval(shardDistributorServiceOperationExpirationInterval)

	return policy
}

// IsValidIDLength checks if id is valid according to its length
func IsValidIDLength(
	id string,
	scope metrics.Scope,
	warnLimit int,
	errorLimit int,
	metricsCounter metrics.MetricIdx,
	domainName string,
	logger log.Logger,
	idTypeViolationTag tag.Tag,
) bool {
	if len(id) > warnLimit {
		scope.IncCounter(metricsCounter)
		logger.Warn("ID length exceeds limit.",
			tag.WorkflowDomainName(domainName),
			tag.Name(id),
			idTypeViolationTag)
	}
	return len(id) <= errorLimit
}

// ToServiceTransientError converts an error to ServiceTransientError
func ToServiceTransientError(err error) error {
	if err == nil || IsServiceTransientError(err) {
		return err
	}
	return yarpcerrors.Newf(yarpcerrors.CodeUnavailable, err.Error())
}

// IsExpectedError checks if an error is expected to happen in normal operation of the system
func IsExpectedError(err error) bool {
	return IsServiceTransientError(err) ||
		IsEntityNotExistsError(err) ||
		errors.As(err, new(*types.WorkflowExecutionAlreadyCompletedError))
}

// IsServiceTransientError checks if the error is a transient error.
func IsServiceTransientError(err error) bool {

	var (
		typesInternalServiceError        *types.InternalServiceError
		typesServiceBusyError            *types.ServiceBusyError
		typesShardOwnershipLostError     *types.ShardOwnershipLostError
		typesTaskListNotOwnedByHostError *cadence_errors.TaskListNotOwnedByHostError
		yarpcErrorsStatus                *yarpcerrors.Status
	)

	switch {
	case errors.As(err, &typesInternalServiceError):
		return true
	case errors.As(err, &typesServiceBusyError):
		return true
	case errors.As(err, &typesShardOwnershipLostError):
		return true
	case errors.As(err, &typesTaskListNotOwnedByHostError):
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

// IsEntityNotExistsError checks if the error is an entity not exists error.
func IsEntityNotExistsError(err error) bool {
	_, ok := err.(*types.EntityNotExistsError)
	return ok
}

// IsServiceBusyError checks if the error is a service busy error.
func IsServiceBusyError(err error) bool {
	switch err.(type) {
	case *types.ServiceBusyError:
		return true
	}
	return false
}

// IsContextTimeoutError checks if the error is context timeout error
func IsContextTimeoutError(err error) bool {
	switch err := err.(type) {
	case *types.InternalServiceError:
		return err.Message == context.DeadlineExceeded.Error()
	}
	return err == context.DeadlineExceeded || yarpcerrors.IsDeadlineExceeded(err)
}

// IsValidContext checks that the thrift context is not expired on cancelled.
// Returns nil if the context is still valid. Otherwise, returns the result of
// ctx.Err()
func IsValidContext(ctx context.Context) error {
	ch := ctx.Done()
	if ch != nil {
		select {
		case <-ch:
			return ctx.Err()
		default:
			// go to the next line
		}
	}

	deadline, ok := ctx.Deadline()
	if ok && time.Until(deadline) < contextExpireThreshold {
		return context.DeadlineExceeded
	}
	return nil
}

// GenerateRandomString is used for generate test string
func GenerateRandomString(n int) string {
	if n <= 0 {
		return ""
	}

	letterRunes := []rune("random")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// ValidateRetryPolicy validates a retry policy
func ValidateRetryPolicy(policy *types.RetryPolicy) error {
	if policy == nil {
		// nil policy is valid which means no retry
		return nil
	}
	if policy.GetInitialIntervalInSeconds() <= 0 {
		return &types.BadRequestError{Message: "InitialIntervalInSeconds must be greater than 0 on retry policy."}
	}
	if policy.GetBackoffCoefficient() < 1 {
		return &types.BadRequestError{Message: "BackoffCoefficient cannot be less than 1 on retry policy."}
	}
	if policy.GetMaximumIntervalInSeconds() < 0 {
		return &types.BadRequestError{Message: "MaximumIntervalInSeconds cannot be less than 0 on retry policy."}
	}
	if policy.GetMaximumIntervalInSeconds() > 0 && policy.GetMaximumIntervalInSeconds() < policy.GetInitialIntervalInSeconds() {
		return &types.BadRequestError{Message: "MaximumIntervalInSeconds cannot be less than InitialIntervalInSeconds on retry policy."}
	}
	if policy.GetMaximumAttempts() < 0 {
		return &types.BadRequestError{Message: "MaximumAttempts cannot be less than 0 on retry policy."}
	}
	if policy.GetExpirationIntervalInSeconds() < 0 {
		return &types.BadRequestError{Message: "ExpirationIntervalInSeconds cannot be less than 0 on retry policy."}
	}
	if policy.GetMaximumAttempts() == 0 && policy.GetExpirationIntervalInSeconds() == 0 {
		return &types.BadRequestError{Message: "MaximumAttempts and ExpirationIntervalInSeconds are both 0. At least one of them must be specified."}
	}
	return nil
}

// GetSizeOfMapStringToByteArray get size of map[string][]byte
func GetSizeOfMapStringToByteArray(input map[string][]byte) int {
	if input == nil {
		return 0
	}

	res := 0
	for k, v := range input {
		res += len(k) + len(v)
	}
	return res + golandMapReserverNumberOfBytes
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

// IntersectionStringSlice get the intersection of 2 string slices
func IntersectionStringSlice(a, b []string) []string {
	var result []string
	m := make(map[string]struct{})
	for _, item := range a {
		m[item] = struct{}{}
	}
	for _, item := range b {
		if _, ok := m[item]; ok {
			result = append(result, item)
		}
	}
	return result
}
