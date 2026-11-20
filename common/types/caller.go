package types

import (
	"context"
)

const (
	CallerTypeHeaderName = "cadence-caller-type" // need to define it here due to circular dependency with common
)

type CallerType string

const (
	CallerTypeUnknown  CallerType = "unknown"
	CallerTypeCLI      CallerType = "cli"
	CallerTypeUI       CallerType = "ui"
	CallerTypeSDK      CallerType = "sdk"
	CallerTypeInternal CallerType = "internal"
)

// CallerInfo captures request source information for observability and resource management.
//
// Intent:
//   - Track the source/origin/actor of API requests (CLI, UI, SDK, internal service calls, etc.)
//   - Enable client-specific behavior and resource allocation decisions
//   - Support future extensibility for additional caller metadata (e.g., identity, version)
//
// Consumers:
//   - Logging and audit systems for request attribution
//   - Metrics and monitoring for client-specific observability
//   - Rate limiting and resource management based on caller information
//
// Lifecycle:
//   - Should be set early in request processing, typically after authentication
//   - Expected for external API calls (CLI, UI, SDK)
//   - May be absent for internal service-to-service calls or unauthenticated endpoints
//   - Set by authentication/authorization middleware or API gateway components
type CallerInfo struct {
	callerType CallerType
}

// NewCallerInfo creates a new CallerInfo
func NewCallerInfo(callerType CallerType) CallerInfo {
	return CallerInfo{callerType: callerType}
}

// GetCallerType returns the CallerType
func (c CallerInfo) GetCallerType() CallerType {
	return c.callerType
}

type callerInfoContextKey string

const callerInfoKey = callerInfoContextKey("caller-info")

func (c CallerType) String() string {
	return string(c)
}

// ParseCallerType converts a string to CallerType
// Returns CallerTypeUnknown if s is empty
func ParseCallerType(s string) CallerType {
	if s == "" {
		return CallerTypeUnknown
	}
	return CallerType(s)
}

// ContextWithCallerInfo adds CallerInfo to context
func ContextWithCallerInfo(ctx context.Context, callerInfo CallerInfo) context.Context {
	return context.WithValue(ctx, callerInfoKey, callerInfo)
}

// GetCallerInfoFromContext retrieves CallerInfo from context
// Returns CallerInfo with CallerTypeUnknown if not set in context
func GetCallerInfoFromContext(ctx context.Context) CallerInfo {
	if ctx == nil {
		return NewCallerInfo(CallerTypeUnknown)
	}
	if callerInfo, ok := ctx.Value(callerInfoKey).(CallerInfo); ok {
		return callerInfo
	}
	return NewCallerInfo(CallerTypeUnknown)
}

// NewCallerInfoFromTransportHeaders extracts CallerInfo from transport headers
// This is used by middleware to extract caller information from incoming requests
func NewCallerInfoFromTransportHeaders(headers interface{ Get(string) (string, bool) }) CallerInfo {
	callerTypeStr, _ := headers.Get(CallerTypeHeaderName)

	// Future: add more header extractions here
	// version, _ := headers.Get("cadence-client-version")
	// identity, _ := headers.Get("cadence-client-identity")

	return NewCallerInfo(ParseCallerType(callerTypeStr))
}

// GetContextWithCallerInfoFromHeaders extracts CallerInfo from transport headers and adds it to the context
func GetContextWithCallerInfoFromHeaders(ctx context.Context, headers interface{ Get(string) (string, bool) }) context.Context {
	return ContextWithCallerInfo(ctx, NewCallerInfoFromTransportHeaders(headers))
}
