package log

import (
	"fmt"
	"runtime/debug"

	"github.com/cadence-workflow/shard-manager/common/log/tag"
)

// CapturePanic is used to capture panic, it will log the panic and also return the error through pointer.
// If the panic value is not error then a default error is returned
// We have to use pointer is because in golang: "recover return nil if was not called directly by a deferred function."
// And we have to set the returned error otherwise our handler will return nil as error which is incorrect
// errPanic MUST be the result from calling recover, which MUST be done in a single level deep
// deferred function. The usual way of calling this is:
// - defer func() { log.CapturePanic(recover(), logger, &err) }()
func CapturePanic(errPanic interface{}, logger Logger, retError *error) {
	if errPanic != nil {
		err, ok := errPanic.(error)
		if !ok {
			err = fmt.Errorf("panic object is not error: %#v", errPanic)
		}

		st := string(debug.Stack())

		// This function is called in deferred block and is all over the place.
		// We want the log to point to the line of panic, not this line, or stack of the defer function.
		logger.Helper().Helper().Error("Panic is captured", tag.SysStackTrace(st), tag.Error(err))

		if retError != nil {
			*retError = err
		}
	}
}
