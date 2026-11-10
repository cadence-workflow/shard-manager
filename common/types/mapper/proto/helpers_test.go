package proto

import (
	"testing"
	"time"

	gogo "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func TestTimeToTimestamp(t *testing.T) {
	testTime := time.Unix(10, 10)
	timestamp := timeToTimestamp(&testTime)
	assert.Equal(t, gogo.Timestamp{Seconds: 10, Nanos: 10}, *timestamp)
}

func TestTimeToTimestampNil(t *testing.T) {
	result := timeToTimestamp(nil)
	assert.Nil(t, result)
}
