package log

import "github.com/cadence-workflow/shard-manager/common/log/tag"

//go:generate mockgen -package=$GOPACKAGE -destination=logger_mock.go -self_package=github.com/cadence-workflow/shard-manager/common/log github.com/cadence-workflow/shard-manager/common/log Logger

// Logger is our abstraction for logging
// Usage examples:
//
//	 import "github.com/cadence-workflow/shard-manager/common/log/tag"
//	 1) logger = logger.WithTags(
//	         tag.WorkflowNextEventID( 123),
//	         tag.WorkflowActionWorkflowStarted,
//	         tag.WorkflowDomainID("test-domain-id"))
//	    logger.Info("hello world")
//	 2) logger.Info("hello world",
//	         tag.WorkflowNextEventID( 123),
//	         tag.WorkflowActionWorkflowStarted,
//	         tag.WorkflowDomainID("test-domain-id"))
//		   )
//	 Note: msg should be static, it is not recommended to use fmt.Sprintf() for msg.
//	       Anything dynamic should be tagged.
type Logger interface {
	Debugf(msg string, args ...any)
	Debug(msg string, tags ...tag.Tag)
	Info(msg string, tags ...tag.Tag)
	Warn(msg string, tags ...tag.Tag)
	Error(msg string, tags ...tag.Tag)
	Fatal(msg string, tags ...tag.Tag)
	WithTags(tags ...tag.Tag) Logger
	SampleInfo(msg string, sampleRate int, tags ...tag.Tag)
	DebugOn() bool
	// Helper returns a logger that will skip one more level in stack trace. This is helpful for layered architecture, when you want to point to a business logic error, instead of pointing to the wrapped generated level.
	Helper() Logger
}

type noop struct{}

// NewNoop return a noop logger
func NewNoop() Logger {
	return &noop{}
}

func (n *noop) Debugf(msg string, args ...any)                         {}
func (n *noop) Debug(msg string, tags ...tag.Tag)                      {}
func (n *noop) Info(msg string, tags ...tag.Tag)                       {}
func (n *noop) Warn(msg string, tags ...tag.Tag)                       {}
func (n *noop) Error(msg string, tags ...tag.Tag)                      {}
func (n *noop) Fatal(msg string, tags ...tag.Tag)                      {}
func (n *noop) SampleInfo(msg string, sampleRate int, tags ...tag.Tag) {}
func (n *noop) WithTags(tags ...tag.Tag) Logger {
	return n
}
func (n *noop) DebugOn() bool {
	return true
}
func (n *noop) Helper() Logger { return &noop{} }
