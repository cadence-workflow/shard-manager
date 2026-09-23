// Copyright (c) 2017-2020 Uber Technologies Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package types

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// AccessDeniedError is an internal type (TBD...)
// TODO(c-warren): Move to common/types/errors.go
type AccessDeniedError struct {
	Message string `json:"message,required"`
}

// BadRequestError is an internal type (TBD...)
// TODO(c-warren): Move to common/types/errors.go
type BadRequestError struct {
	Message string `json:"message,required"`
}

// DataBlob is an internal type (TBD...)
type DataBlob struct {
	EncodingType *EncodingType `json:"EncodingType,omitempty"`
	Data         []byte        `json:"Data,omitempty"`
}

// GetEncodingType is an internal getter (TBD...)
func (v *DataBlob) GetEncodingType() (o EncodingType) {
	if v != nil && v.EncodingType != nil {
		return *v.EncodingType
	}
	return
}

// GetData is an internal getter (TBD...)
func (v *DataBlob) GetData() (o []byte) {
	if v != nil && v.Data != nil {
		return v.Data
	}
	return
}

func (v *DataBlob) DeepCopy() *DataBlob {
	if v == nil {
		return nil
	}

	res := &DataBlob{
		EncodingType: v.EncodingType,
	}

	if v.Data != nil {
		res.Data = make([]byte, len(v.Data))
		copy(res.Data, v.Data)
	}

	return res
}

// ByteSize returns the approximate memory used in bytes
func (v *DataBlob) ByteSize() uint64 {
	if v == nil {
		return 0
	}

	size := uint64(unsafe.Sizeof(*v))
	size += v.EncodingType.ByteSize()
	size += uint64(len(v.Data))
	return size
}

// EncodingType is an internal type (TBD...)
type EncodingType int32

// Ptr is a helper function for getting pointer value
func (e EncodingType) Ptr() *EncodingType {
	return &e
}

// String returns a readable string representation of EncodingType.
func (e EncodingType) String() string {
	w := int32(e)
	switch w {
	case 0:
		return "ThriftRW"
	case 1:
		return "JSON"
	}
	return fmt.Sprintf("EncodingType(%d)", w)
}

// UnmarshalText parses enum value from string representation
func (e *EncodingType) UnmarshalText(value []byte) error {
	switch s := strings.ToUpper(string(value)); s {
	case "THRIFTRW":
		*e = EncodingTypeThriftRW
		return nil
	case "JSON":
		*e = EncodingTypeJSON
		return nil
	default:
		val, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return fmt.Errorf("unknown enum value %q for %q: %v", s, "EncodingType", err)
		}
		*e = EncodingType(val)
		return nil
	}
}

// MarshalText encodes EncodingType to text.
func (e EncodingType) MarshalText() ([]byte, error) {
	return []byte(e.String()), nil
}

const (
	// EncodingTypeThriftRW is an option for EncodingType
	EncodingTypeThriftRW EncodingType = iota
	// EncodingTypeJSON is an option for EncodingType
	EncodingTypeJSON
)

// ByteSize returns the approximate memory used in bytes
func (e *EncodingType) ByteSize() uint64 {
	if e == nil {
		return 0
	}

	return uint64(unsafe.Sizeof(*e))
}

// EntityNotExistsError is an internal type (TBD...)
type EntityNotExistsError struct {
	Message        string   `json:"message,required"`
	CurrentCluster string   `json:"currentCluster,omitempty"`
	ActiveCluster  string   `json:"activeCluster,omitempty"`
	ActiveClusters []string `json:"activeClusters,omitempty"`
}

// InternalServiceError is an internal type (TBD...)
type InternalServiceError struct {
	Message string `json:"message,required"`
}

// GetMessage is an internal getter (TBD...)
func (v *InternalServiceError) GetMessage() (o string) {
	if v != nil {
		return v.Message
	}
	return
}

// ServiceBusyError is an internal type (TBD...)
type ServiceBusyError struct {
	Message string `json:"message,required"`
	Reason  string `json:"reason,omitempty"`
}
