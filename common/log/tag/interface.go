package tag

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Tag is the interface for logging system
type Tag struct {
	// keep this field private
	field zap.Field
}

// Field returns a zap field
func (t *Tag) Field() zap.Field {
	return t.field
}

func newStringTag(key string, value string) Tag {
	return Tag{
		field: zap.String(key, value),
	}
}

func newStringsTag(key string, value []string) Tag {
	return Tag{
		field: zap.Strings(key, value),
	}
}

func newInt64(key string, value int64) Tag {
	return Tag{
		field: zap.Int64(key, value),
	}
}

func newInt(key string, value int) Tag {
	return Tag{
		field: zap.Int(key, value),
	}
}

func newBoolTag(key string, value bool) Tag {
	return Tag{
		field: zap.Bool(key, value),
	}
}

func newErrorTag(key string, value error) Tag {
	// NOTE zap already chosen "error" as key
	return Tag{
		field: zap.Error(value),
	}
}

func newDurationTag(key string, value time.Duration) Tag {
	return Tag{
		field: zap.Duration(key, value),
	}
}

func newObjectTag(key string, value interface{}) Tag {
	return Tag{
		field: zap.String(key, fmt.Sprintf("%v", value)),
	}
}

func newPredefinedStringTag(key string, value string) Tag {
	return Tag{
		field: zap.String(key, value),
	}
}

func newPredefinedDynamicTag(key string, value interface{}) Tag {
	return Tag{
		field: zap.Any(key, value),
	}
}
