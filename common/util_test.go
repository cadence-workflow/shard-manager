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
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		"ShardOwnershipLostError": {
			err:  &types.ShardOwnershipLostError{},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, c.want, IsServiceTransientError(c.err))
		})
	}
}

func TestIsExpectedError(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want bool
	}{
		"An error": {
			err:  assert.AnError,
			want: false,
		},
		"Transient error": {
			err:  &types.ServiceBusyError{},
			want: true,
		},
		"Entity not exists error": {
			err:  &types.EntityNotExistsError{},
			want: true,
		},
		"Already completed error": {
			err:  &types.WorkflowExecutionAlreadyCompletedError{},
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, c.want, IsExpectedError(c.err))
		})
	}
}

func TestIsContextTimeoutError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(50 * time.Millisecond)
	require.True(t, IsContextTimeoutError(ctx.Err()))
	require.True(t, IsContextTimeoutError(&types.InternalServiceError{Message: ctx.Err().Error()}))

	yarpcErr := yarpcerrors.DeadlineExceededErrorf("yarpc deadline exceeded")
	require.True(t, IsContextTimeoutError(yarpcErr))

	require.False(t, IsContextTimeoutError(errors.New("some random error")))

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	require.False(t, IsContextTimeoutError(ctx.Err()))
}

func TestToServiceTransientError(t *testing.T) {
	t.Run("it converts nil", func(t *testing.T) {
		assert.NoError(t, ToServiceTransientError(nil))
	})

	t.Run("it keeps transient errors", func(t *testing.T) {
		err := &types.InternalServiceError{}
		assert.Equal(t, err, ToServiceTransientError(err))
		assert.True(t, IsServiceTransientError(ToServiceTransientError(err)))
	})

	t.Run("it converts errors to transient errors", func(t *testing.T) {
		err := fmt.Errorf("error")
		assert.True(t, IsServiceTransientError(ToServiceTransientError(err)))
	})
}

func TestIntersectionStringSlice(t *testing.T) {
	t.Run("it returns all items", func(t *testing.T) {
		a := []string{"a", "b", "c"}
		b := []string{"a", "b", "c"}
		c := IntersectionStringSlice(a, b)
		assert.ElementsMatch(t, []string{"a", "b", "c"}, c)
	})

	t.Run("it returns no item", func(t *testing.T) {
		a := []string{"a", "b", "c"}
		b := []string{"d", "e", "f"}
		c := IntersectionStringSlice(a, b)
		assert.ElementsMatch(t, []string{}, c)
	})

	t.Run("it returns intersection", func(t *testing.T) {
		a := []string{"a", "b", "c"}
		b := []string{"c", "b", "f"}
		c := IntersectionStringSlice(a, b)
		assert.ElementsMatch(t, []string{"c", "b"}, c)
	})
}

func TestAwaitWaitGroup(t *testing.T) {
	t.Run("wait group done before timeout", func(t *testing.T) {
		var wg sync.WaitGroup

		wg.Add(1)
		wg.Done()

		got := AwaitWaitGroup(&wg, time.Second)
		require.True(t, got)
	})

	t.Run("wait group done after timeout", func(t *testing.T) {
		var (
			wg    sync.WaitGroup
			doneC = make(chan struct{})
		)

		wg.Add(1)
		go func() {
			<-doneC
			wg.Done()
		}()

		got := AwaitWaitGroup(&wg, time.Microsecond)
		require.False(t, got)

		doneC <- struct{}{}
		close(doneC)
	})
}

func TestGenerateRandomString(t *testing.T) {
	for input, wantSize := range map[int]int{
		-1: 0,
		0:  0,
		10: 10,
	} {
		t.Run(fmt.Sprintf("%d", input), func(t *testing.T) {
			got := GenerateRandomString(input)
			require.Len(t, got, wantSize)
		})
	}
}

func TestIsValidContext(t *testing.T) {
	t.Run("background context", func(t *testing.T) {
		require.NoError(t, IsValidContext(context.Background()))
	})
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := IsValidContext(ctx)
		require.Error(t, got)
		require.ErrorIs(t, got, context.Canceled)
	})
	t.Run("deadline exceeded context", func(t *testing.T) {
		ctx, _ := context.WithTimeout(context.Background(), -time.Second)
		got := IsValidContext(ctx)
		require.Error(t, got)
		require.ErrorIs(t, got, context.DeadlineExceeded)
	})
	t.Run("context with deadline exceeded contextExpireThreshold", func(t *testing.T) {
		ctx, _ := context.WithTimeout(context.Background(), contextExpireThreshold/2)
		got := IsValidContext(ctx)
		require.Error(t, got)
		require.ErrorIs(t, got, context.DeadlineExceeded, "context.DeadlineExceeded should be returned, because context timeout is not later than now + contextExpireThreshold")
	})
	t.Run("valid context", func(t *testing.T) {
		ctx, _ := context.WithTimeout(context.Background(), contextExpireThreshold*2)
		require.NoError(t, IsValidContext(ctx), "nil should be returned, because context timeout is later than now + contextExpireThreshold")
	})
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
