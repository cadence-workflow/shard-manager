package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordWriter(t *testing.T) {
	original := []byte("test-data")

	t.Run("no compression when empty", func(t *testing.T) {
		writer, err := NewRecordWriter("")
		require.NoError(t, err)

		out, err := writer.Write(original)
		require.NoError(t, err)
		assert.Equal(t, original, out)
	})

	t.Run("no compression when none", func(t *testing.T) {
		writer, err := NewRecordWriter("none")
		require.NoError(t, err)

		out, err := writer.Write(original)
		require.NoError(t, err)
		assert.Equal(t, original, out)
	})

	t.Run("snappy compression", func(t *testing.T) {
		writer, err := NewRecordWriter(CompressionSnappy)
		require.NoError(t, err)

		out, err := writer.Write(original)
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.NotEqual(t, original, out)

		decompressed, err := Decompress(out)
		require.NoError(t, err)
		assert.Equal(t, original, decompressed)
	})

	t.Run("unsupported compression", func(t *testing.T) {
		writer, err := NewRecordWriter("unsupported")
		require.Error(t, err)
		assert.Nil(t, writer)
	})
}

func TestDecompress(t *testing.T) {
	t.Run("Empty data", func(t *testing.T) {
		decompressed, err := Decompress([]byte{})
		require.NoError(t, err)
		assert.Empty(t, decompressed)
	})

	t.Run("Nil data", func(t *testing.T) {
		decompressed, err := Decompress(nil)
		require.NoError(t, err)
		assert.Nil(t, decompressed)
	})

	t.Run("Uncompressed data", func(t *testing.T) {
		uncompressed := []byte(`{"status":"ACTIVE"}`)

		result, err := Decompress(uncompressed)
		require.NoError(t, err)
		assert.Equal(t, uncompressed, result, "Uncompressed data is returned as-is")

		var status map[string]string
		err = json.Unmarshal(result, &status)
		require.NoError(t, err)
		assert.Equal(t, "ACTIVE", status["status"])
	})

	t.Run("Compressed data", func(t *testing.T) {
		original := []byte(`{"status":"DRAINING"}`)
		writer, err := NewRecordWriter(CompressionSnappy)
		require.NoError(t, err)

		compressed, err := writer.Write(original)
		require.NoError(t, err)

		result, err := Decompress(compressed)
		require.NoError(t, err)
		assert.Equal(t, original, result)

		var status map[string]string
		err = json.Unmarshal(result, &status)
		require.NoError(t, err)
		assert.Equal(t, "DRAINING", status["status"])
	})
}
