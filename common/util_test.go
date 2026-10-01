// The MIT License (MIT)
//
// Copyright (c) 2020 Uber Technologies, Inc.
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

package common

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/shard-manager/common/types"
)

func TestIsServiceTransientError(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want bool
	}{
		"ContextTimeout": {
			err:  context.DeadlineExceeded,
			want: false,
		},
		"YARPCCanceled": {
			err:  yarpcerrors.CancelledErrorf("connection closing"),
			want: true,
		},
		"YARPCDeadlineExceeded": {
			err:  yarpcerrors.DeadlineExceededErrorf("yarpc deadline exceeded"),
			want: false,
		},
		"YARPCUnavailable": {
			err:  yarpcerrors.UnavailableErrorf("yarpc unavailable"),
			want: true,
		},
		"YARPCUnavailable wrapped": {
			err:  fmt.Errorf("wrapped err: %w", yarpcerrors.UnavailableErrorf("yarpc unavailable")),
			want: true,
		},
		"YARPCUnknown": {
			err:  yarpcerrors.UnknownErrorf("yarpc unknown"),
			want: true,
		},
		"YARPCInternal": {
			err:  yarpcerrors.InternalErrorf("yarpc internal"),
			want: true,
		},
		"ContextCancel": {
			err:  context.Canceled,
			want: false,
		},
		"ServiceBusyError": {
			err:  &types.ServiceBusyError{},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, c.want, IsServiceTransientError(c.err))
		})
	}
}

func TestDurationToDays(t *testing.T) {
	for duration, want := range map[time.Duration]int32{
		0:              0,
		time.Hour:      0,
		24 * time.Hour: 1,
		25 * time.Hour: 1,
		48 * time.Hour: 2,
	} {
		t.Run(duration.String(), func(t *testing.T) {
			got := DurationToDays(duration)
			require.Equal(t, want, got)
		})
	}
}

func TestDurationToSeconds(t *testing.T) {
	for duration, want := range map[time.Duration]int64{
		0:                           0,
		time.Second:                 1,
		time.Second + time.Second/2: 1,
		2 * time.Second:             2,
	} {
		t.Run(duration.String(), func(t *testing.T) {
			got := DurationToSeconds(duration)
			require.Equal(t, want, got)
		})
	}
}

func TestDaysToDuration(t *testing.T) {
	for days, want := range map[int32]time.Duration{
		0: 0,
		1: 24 * time.Hour,
		2: 48 * time.Hour,
	} {
		t.Run(strconv.Itoa(int(days)), func(t *testing.T) {
			got := DaysToDuration(days)
			require.Equal(t, want, got)
		})
	}
}

func TestSecondsToDuration(t *testing.T) {
	for seconds, want := range map[int64]time.Duration{
		0: 0,
		1: time.Second,
		2: 2 * time.Second,
	} {
		t.Run(strconv.Itoa(int(seconds)), func(t *testing.T) {
			got := SecondsToDuration(seconds)
			require.Equal(t, want, got)
		})
	}
}
