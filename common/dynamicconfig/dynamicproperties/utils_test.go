package dynamicproperties

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertDynamicConfigMapPropertyToIntMap(t *testing.T) {
	dcValue := make(map[string]interface{})
	for idx, value := range []interface{}{int(0), int32(1), int64(2), float64(3.0)} {
		dcValue[strconv.Itoa(idx)] = value
	}

	intMap, err := ConvertDynamicConfigMapPropertyToIntMap(dcValue)
	require.NoError(t, err)
	require.Len(t, intMap, 4)
	for i := 0; i != 4; i++ {
		require.Equal(t, i, intMap[i])
	}
}
