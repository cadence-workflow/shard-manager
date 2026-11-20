package hash

import (
	"bytes"
	"testing"
)

func TestFingerprint64(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{
			name:  "empty input",
			input: []byte{},
			want:  []byte{0x9a, 0xe1, 0x6a, 0x3b, 0x2f, 0x90, 0x40, 0x4f},
		},
		{
			name:  "tasklist name",
			input: []byte("my-task-list"),
			want:  []byte{0x83, 0xee, 0x86, 0x1a, 0xac, 0x65, 0x53, 0x62},
		},
		{
			name:  "partitioned tasklist name",
			input: []byte("/__cadence_sys/my-task-list/0"),
			want:  []byte{0x79, 0xe2, 0x85, 0xf3, 0x5c, 0x92, 0x39, 0x44},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fingerprint64(tt.input)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Fingerprint64(%q) = %x, want %x", tt.input, got, tt.want)
			}
			if len(got) != fingerprintSize {
				t.Errorf("Fingerprint64() length = %d, want %d", len(got), fingerprintSize)
			}
		})
	}
}

func TestFingerprint64_IsDeterministic(t *testing.T) {
	input := []byte("my-task-list")
	first := Fingerprint64(input)
	second := Fingerprint64(input)

	if !bytes.Equal(first, second) {
		t.Fatalf("Fingerprint64() changed between calls: %x != %x", first, second)
	}

	input[0] = 'M'
	if bytes.Equal(first, Fingerprint64(input)) {
		t.Error("Fingerprint64() returned the same key after input changed")
	}
}

func TestFingerprint64String_MatchesBytes(t *testing.T) {
	input := "my-task-list"
	if got, want := Fingerprint64String(input), Fingerprint64([]byte(input)); !bytes.Equal(got, want) {
		t.Errorf("Fingerprint64String(%q) = %x, want %x", input, got, want)
	}
}

func TestIsMinFingerprint(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		want bool
	}{
		{name: "exact 8 zero bytes", key: make([]byte, 8), want: true},
		{name: "nil", key: nil, want: false},
		{name: "empty", key: []byte{}, want: false},
		{name: "7 zero bytes", key: make([]byte, 7), want: false},
		{name: "9 zero bytes", key: make([]byte, 9), want: false},
		{name: "non-zero 8 bytes", key: []byte{0, 0, 0, 0, 0, 0, 0, 1}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMinFingerprint(tt.key); got != tt.want {
				t.Errorf("IsMinFingerprint(%x) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
