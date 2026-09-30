package hostname

import (
	"errors"
	"fmt"
	"strings"
)

const MaxLength = 128

// Normalize makes a hostname safe as an etcd path segment and as the host
// portion of hostname@uuid executor IDs.
func Normalize(raw string) string {
	raw = strings.ReplaceAll(raw, "/", "_")
	raw = strings.ReplaceAll(raw, "@", "_")
	if len(raw) > MaxLength {
		return raw[:MaxLength]
	}
	return raw
}

// Validate rejects hostnames that cannot survive an etcd key round trip or that
// would break hostname@uuid matching. Empty names are invalid
func Validate(name string) error {
	if name == "" {
		return errors.New("hostname must not be empty")
	}
	if len(name) > MaxLength {
		return fmt.Errorf("hostname %q exceeds %d bytes", name, MaxLength)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("hostname %q must not contain '/'", name)
	}
	if strings.Contains(name, "@") {
		return fmt.Errorf("hostname %q must not contain '@'", name)
	}
	return nil
}
