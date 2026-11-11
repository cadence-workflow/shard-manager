package types

import (
	"testing"
)

func TestError(t *testing.T) {
	const message = "test"
	tests := []struct {
		name string
		err  error
	}{
		{name: "access denied", err: AccessDeniedError{Message: message}},
		{name: "bad request", err: BadRequestError{Message: message}},
		{name: "entity not found", err: EntityNotExistsError{Message: message}},
		{name: "internal service", err: InternalServiceError{Message: message}},
		{name: "service busy", err: ServiceBusyError{Message: message}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != message {
				t.Fatalf("Error() = %q, want %q", got, message)
			}
		})
	}
}
