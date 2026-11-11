package types

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestDataBlobDeepCopy(t *testing.T) {
	tests := []struct {
		name  string
		input *DataBlob
	}{
		{
			name:  "nil",
			input: nil,
		},
		{
			name:  "empty",
			input: &DataBlob{},
		},
		{
			name: "thrift ok",
			input: &DataBlob{
				EncodingType: EncodingTypeThriftRW.Ptr(),
				Data:         []byte("some thrift data"),
			},
		},
		{
			name: "json ok",
			input: &DataBlob{
				EncodingType: EncodingTypeJSON.Ptr(),
				Data:         []byte("some json data"),
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.DeepCopy()
			assert.Equal(t, tc.input, got)
			if tc.input != nil && tc.input.Data != nil && identicalByteArray(tc.input.Data, got.Data) {
				t.Error("expected DeepCopy to return a new data slice")
			}
		})
	}
}

// identicalByteArray returns true if a and b are the same slice, false otherwise.
func identicalByteArray(a, b []byte) bool {
	return len(a) == len(b) && unsafe.SliceData(a) == unsafe.SliceData(b)
}
