package log

// Option is used to set options for the logger.
type Option func(impl *loggerImpl)

// WithSampleFunc sets the sampling function for the logger.
func WithSampleFunc(fn func(int) bool) Option {
	return func(impl *loggerImpl) {
		impl.sampleLocalFn = fn
	}
}
