package common

import (
	"time"
)

// Returns a pointer to the given value.
// todo (david.porter) Remove the remaining helpers here which are now obsolete
func Ptr[T any](v T) *T {
	return &v
}

// IntPtr makes a copy and returns the pointer to an int.
func IntPtr(v int) *int {
	return &v
}

// Int16Ptr makes a copy and returns the pointer to an int16.
func Int16Ptr(v int16) *int16 {
	return &v
}

// Int32Ptr makes a copy and returns the pointer to an int32.
func Int32Ptr(v int32) *int32 {
	return &v
}

// Int64Ptr makes a copy and returns the pointer to an int64.
func Int64Ptr(v int64) *int64 {
	return &v
}

// Uint32Ptr makes a copy and returns the pointer to a uint32.
func Uint32Ptr(v uint32) *uint32 {
	return &v
}

// Uint64Ptr makes a copy and returns the pointer to a uint64.
func Uint64Ptr(v uint64) *uint64 {
	return &v
}

// Float64Ptr makes a copy and returns the pointer to an int64.
func Float64Ptr(v float64) *float64 {
	return &v
}

// BoolPtr makes a copy and returns the pointer to a bool.
func BoolPtr(v bool) *bool {
	return &v
}

// StringPtr makes a copy and returns the pointer to a string.
func StringPtr(v string) *string {
	return &v
}

// TimeNowNanosPtr returns an int64 ptr to current time in unix nanos
func TimeNowNanosPtr() *int64 {
	v := time.Now().UnixNano()
	return &v
}

// TimePtr makes a copy and returns the pointer to a time
func TimePtr(v time.Time) *time.Time {
	return &v
}

// DurationPtr makes a copy and returns the pointer to a duration
func DurationPtr(v time.Duration) *time.Duration {
	return &v
}

// StringDefault returns value if string pointer is set otherwise default value of string
func StringDefault(v *string) string {
	var defaultString string
	if v == nil {
		return defaultString
	}
	return *v
}

// Int32Default returns value if int32 pointer is set otherwise default value of int32
func Int32Default(v *int32) int32 {
	var defaultInt32 int32
	if v == nil {
		return defaultInt32
	}
	return *v
}

// Int64Default returns value if int64 pointer is set otherwise default value of int64
func Int64Default(v *int64) int64 {
	var defaultInt64 int64
	if v == nil {
		return defaultInt64
	}
	return *v
}

// BoolDefault returns value if bool pointer is set otherwise default value of bool
func BoolDefault(v *bool) bool {
	var defaultBool bool
	if v == nil {
		return defaultBool
	}
	return *v
}
