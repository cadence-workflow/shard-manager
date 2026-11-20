package errors

import (
	"context"
	"errors"
	"math/rand"

	"github.com/cadence-workflow/shard-manager/common/types"
)

var (
	// ErrFakeServiceBusy is a fake service busy error.
	ErrFakeServiceBusy = &types.ServiceBusyError{Message: "Fake Service Busy Error."}
	// ErrFakeInternalService is a fake internal service error.
	ErrFakeInternalService = &types.InternalServiceError{Message: "Fake Internal Service Error."}
	// ErrFakeTimeout is a fake timeout error.
	ErrFakeTimeout = context.DeadlineExceeded
	// ErrFakeUnhandled is a fake unhandled error.
	ErrFakeUnhandled = errors.New("fake unhandled error")
)

var (
	fakeErrors = []error{
		ErrFakeServiceBusy,
		ErrFakeInternalService,
		ErrFakeTimeout,
		ErrFakeUnhandled,
	}
)

// ShouldForwardCall determines if the call should be forward to the underlying
// client given the fake error generated
func ShouldForwardCall(
	err error,
) bool {
	if err == nil {
		return true
	}

	if err == ErrFakeTimeout || err == ErrFakeUnhandled {
		// forward the call with 50% chance
		return rand.Intn(2) == 0
	}

	return false
}

// GenerateFakeError generates a random fake error
func GenerateFakeError(
	errorRate float64,
) error {
	if rand.Float64() < errorRate {
		return fakeErrors[rand.Intn(len(fakeErrors))]
	}

	return nil
}
