package smctl

import (
	"encoding/json"
	"fmt"
	"io"
)

func writeIndentedJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
