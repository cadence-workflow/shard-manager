// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

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

package ephemeralassigner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/cadence-workflow/shard-manager/common/types"
)

// processFnFromMap returns an ephemeralAssignmentBatchFn that resolves shard keys from a
// fixed map, returning an empty map (and no error) for any key not present.
func processFnFromMap(results map[string]*types.GetShardOwnerResponse) ephemeralAssignmentBatchFn {
	return func(_ context.Context, _ string, shardKeys []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
		out := make(map[string]*types.GetShardOwnerResponse, len(shardKeys))
		for _, k := range shardKeys {
			if v, ok := results[k]; ok {
				out[k] = v
			}
		}
		return out, nil, nil
	}
}

func TestShardBatcher_Submit(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		name      string
		batchFn   ephemeralAssignmentBatchFn
		namespace string
		shardKey  string
		// ctxFn builds the context used for the Submit call; defaults to context.Background().
		ctxFn      func() context.Context
		wantErr    bool
		wantErrIs  error
		wantErrMsg string
		wantOwner  string
	}{
		{
			name: "single request resolved immediately",
			batchFn: processFnFromMap(map[string]*types.GetShardOwnerResponse{
				"shard-1": {Owner: "exec-1", Namespace: "ns"},
			}),
			namespace: "ns",
			shardKey:  "shard-1",
			wantOwner: "exec-1",
		},
		{
			name: "batch function returns error - propagated to caller",
			batchFn: func(_ context.Context, _ string, _ []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				return nil, nil, errors.New("storage unavailable")
			},
			namespace:  "ns",
			shardKey:   "shard-1",
			wantErr:    true,
			wantErrMsg: "storage unavailable",
		},
		{
			name: "key absent from batch result returns internal error",
			batchFn: func(_ context.Context, _ string, _ []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				return map[string]*types.GetShardOwnerResponse{}, nil, nil
			},
			namespace:  "ns",
			shardKey:   "shard-missing",
			wantErr:    true,
			wantErrMsg: "shard-missing",
		},
		{
			name: "drained key returns ShardDrainedError",
			batchFn: func(_ context.Context, _ string, _ []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				return map[string]*types.GetShardOwnerResponse{}, map[string]struct{}{"shard-drained": {}}, nil
			},
			namespace:  "ns",
			shardKey:   "shard-drained",
			wantErr:    true,
			wantErrMsg: "shard drained ns:shard-drained",
		},
		{
			name:      "context cancelled before submit",
			batchFn:   processFnFromMap(nil),
			namespace: "ns",
			shardKey:  "shard-1",
			ctxFn: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr:   true,
			wantErrIs: context.Canceled,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newShardBatcher(time.Second, tc.batchFn)
			b.Start()
			defer b.Stop()

			ctx := context.Background()
			if tc.ctxFn != nil {
				ctx = tc.ctxFn()
			}

			resp, err := b.Submit(ctx, &types.GetShardOwnerRequest{Namespace: tc.namespace, ShardKey: tc.shardKey})
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
				if tc.wantErrMsg != "" {
					assert.ErrorContains(t, err, tc.wantErrMsg)
				}
				return
			}

			require.NoError(t, err)
			if tc.wantOwner != "" {
				assert.Equal(t, tc.wantOwner, resp.Owner)
			}
		})
	}
}

func TestToBatchResponse_DrainedVsMissing(t *testing.T) {
	tests := []struct {
		name      string
		results   map[string]*types.GetShardOwnerResponse
		drained   map[string]struct{}
		err       error
		namespace string
		shardKey  string
		wantOwner string
		wantErr   error
	}{
		{
			name:      "assigned shard",
			results:   map[string]*types.GetShardOwnerResponse{"shard-1": {Owner: "exec-1"}},
			namespace: "ns",
			shardKey:  "shard-1",
			wantOwner: "exec-1",
		},
		{
			name:      "drained shard",
			results:   map[string]*types.GetShardOwnerResponse{},
			drained:   map[string]struct{}{"shard-1": {}},
			namespace: "ns",
			shardKey:  "shard-1",
			wantErr:   &types.ShardDrainedError{Namespace: "ns", ShardKey: "shard-1"},
		},
		{
			name:      "missing shard is internal error",
			results:   map[string]*types.GetShardOwnerResponse{},
			namespace: "ns",
			shardKey:  "shard-1",
			wantErr:   &types.InternalServiceError{Message: "batch processor returned no result for shard key: shard-1"},
		},
		{
			name:      "batch error wins over drained set",
			drained:   map[string]struct{}{"shard-1": {}},
			err:       errors.New("storage unavailable"),
			namespace: "ns",
			shardKey:  "shard-1",
			wantErr:   errors.New("storage unavailable"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toBatchResponse(tc.results, tc.drained, tc.err, tc.namespace, tc.shardKey)
			if tc.wantErr != nil {
				require.EqualError(t, got.err, tc.wantErr.Error())
				require.Nil(t, got.resp)
				return
			}
			require.NoError(t, got.err)
			require.Equal(t, tc.wantOwner, got.resp.Owner)
		})
	}
}

func TestShardBatcher_MultipleNamespacesIsolated(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		name     string
		results  map[string]*types.GetShardOwnerResponse
		requests []struct {
			namespace string
			shardKey  string
			wantOwner string
		}
	}{
		{
			name: "two namespaces resolved independently",
			results: map[string]*types.GetShardOwnerResponse{
				"shard-a": {Owner: "exec-ns1", Namespace: "ns1"},
				"shard-b": {Owner: "exec-ns2", Namespace: "ns2"},
			},
			requests: []struct {
				namespace string
				shardKey  string
				wantOwner string
			}{
				{namespace: "ns1", shardKey: "shard-a", wantOwner: "exec-ns1"},
				{namespace: "ns2", shardKey: "shard-b", wantOwner: "exec-ns2"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newShardBatcher(time.Second, processFnFromMap(tc.results))
			b.Start()
			defer b.Stop()

			type result struct {
				owner string
				err   error
			}
			got := make([]result, len(tc.requests))
			var wg sync.WaitGroup
			for i, req := range tc.requests {
				wg.Add(1)
				go func(i int, namespace, shardKey string) {
					defer wg.Done()
					resp, err := b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: namespace, ShardKey: shardKey})
					if err == nil {
						got[i] = result{owner: resp.Owner}
					} else {
						got[i] = result{err: err}
					}
				}(i, req.namespace, req.shardKey)
			}

			wg.Wait()

			for i, req := range tc.requests {
				require.NoError(t, got[i].err)
				assert.Equal(t, req.wantOwner, got[i].owner)
			}
		})
	}
}

func TestShardBatcher_ErrorPropagatedToAllCallers(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		name       string
		batchErr   error
		numCallers int
	}{
		{
			name:       "error propagated to all concurrent callers",
			batchErr:   errors.New("storage unavailable"),
			numCallers: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Block the first batch so all callers queue up together.
			firstCall := make(chan struct{})
			batchFn := func(_ context.Context, _ string, _ []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				<-firstCall
				return nil, nil, tc.batchErr
			}

			b := newShardBatcher(time.Second, batchFn)
			b.Start()
			defer b.Stop()

			errs := make([]error, tc.numCallers)
			var wg sync.WaitGroup

			// Submit the first request which will trigger an immediate flush.
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[0] = b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: "shard"})
			}()

			// Give the first request time to be picked up and start flushing.
			time.Sleep(10 * time.Millisecond)

			// Submit the remaining requests — they'll queue as pending while the first is inflight.
			for i := 1; i < tc.numCallers; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					_, errs[i] = b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: "shard"})
				}(i)
			}

			// Give pending requests time to enqueue, then unblock.
			time.Sleep(10 * time.Millisecond)
			close(firstCall)

			wg.Wait()

			for i, err := range errs {
				assert.ErrorContains(t, err, tc.batchErr.Error(), "caller %d should receive the batch error", i)
			}
		})
	}
}

func TestShardBatcher_CoalescingBehavior(t *testing.T) {
	defer goleak.VerifyNone(t)

	t.Run("requests arriving during inflight batch are coalesced into the next batch", func(t *testing.T) {
		var mu sync.Mutex
		batchCalls := make([][]string, 0)
		gate := make(chan struct{})
		var callCount atomic.Int32

		batchFn := func(_ context.Context, _ string, shardKeys []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
			call := callCount.Add(1)
			if call == 1 {
				<-gate
			}
			mu.Lock()
			batchCalls = append(batchCalls, shardKeys)
			mu.Unlock()

			out := make(map[string]*types.GetShardOwnerResponse, len(shardKeys))
			for _, k := range shardKeys {
				out[k] = &types.GetShardOwnerResponse{Owner: "exec-1", Namespace: "ns"}
			}
			return out, nil, nil
		}

		b := newShardBatcher(time.Second, batchFn)
		b.Start()
		defer b.Stop()

		var wg sync.WaitGroup

		// Submit first request — triggers immediate flush, which blocks on gate.
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: "shard-1"})
			require.NoError(t, err)
			assert.Equal(t, "exec-1", resp.Owner)
		}()

		// Give the first request time to start flushing.
		time.Sleep(10 * time.Millisecond)

		// Submit more requests while the first flush is in-flight.
		for _, key := range []string{"shard-2", "shard-3", "shard-4"} {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				resp, err := b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: k})
				require.NoError(t, err)
				assert.Equal(t, "exec-1", resp.Owner)
			}(key)
		}

		// Give pending requests time to enqueue.
		time.Sleep(10 * time.Millisecond)

		// Unblock the first flush — pending requests should fire as a single batch.
		close(gate)
		wg.Wait()

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, batchCalls, 2, "expected exactly 2 batch calls: first immediate, second coalesced")
		assert.Len(t, batchCalls[0], 1, "first batch should contain only the initial request")
		assert.Len(t, batchCalls[1], 3, "second batch should contain the 3 coalesced requests")
	})

	t.Run("single request is processed without delay", func(t *testing.T) {
		processed := make(chan struct{})
		batchFn := func(_ context.Context, _ string, shardKeys []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
			close(processed)
			out := make(map[string]*types.GetShardOwnerResponse, len(shardKeys))
			for _, k := range shardKeys {
				out[k] = &types.GetShardOwnerResponse{Owner: "exec-1", Namespace: "ns"}
			}
			return out, nil, nil
		}

		b := newShardBatcher(time.Second, batchFn)
		b.Start()
		defer b.Stop()

		go func() {
			_, _ = b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: "shard-1"})
		}()

		select {
		case <-processed:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("batch was not processed immediately — coalescing should not add delay for the first request")
		}
	})
}

func TestShardBatcher_ConcurrentRequestsBatchedTogether(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		name      string
		numShards int
	}{
		{
			name:      "20 concurrent shards coalesced across batch calls",
			numShards: 20,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]*types.GetShardOwnerResponse, tc.numShards)
			for i := range tc.numShards {
				key := fmt.Sprintf("shard-%d", i)
				results[key] = &types.GetShardOwnerResponse{Owner: "exec-1", Namespace: "ns"}
			}

			var mu sync.Mutex
			maxBatchSize := 0
			gate := make(chan struct{})
			var callCount atomic.Int32

			batchFn := func(_ context.Context, _ string, shardKeys []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				call := callCount.Add(1)
				if call == 1 {
					<-gate
				}

				mu.Lock()
				if len(shardKeys) > maxBatchSize {
					maxBatchSize = len(shardKeys)
				}
				mu.Unlock()
				out := make(map[string]*types.GetShardOwnerResponse, len(shardKeys))
				for _, k := range shardKeys {
					if v, ok := results[k]; ok {
						out[k] = v
					}
				}
				return out, nil, nil
			}

			b := newShardBatcher(time.Second, batchFn)
			b.Start()
			defer b.Stop()

			var wg sync.WaitGroup
			for i := range tc.numShards {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					key := fmt.Sprintf("shard-%d", i)
					resp, err := b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: key})
					require.NoError(t, err)
					assert.Equal(t, "exec-1", resp.Owner)
				}(i)
			}

			// Let requests enqueue while the first flush is blocked.
			time.Sleep(20 * time.Millisecond)
			close(gate)

			wg.Wait()

			mu.Lock()
			defer mu.Unlock()
			assert.Greater(t, maxBatchSize, 1, "expected at least one batch call to receive more than one shard")
		})
	}
}

func TestShardBatcher_StopDrainsAndCancelsRemainingRequests(t *testing.T) {
	defer goleak.VerifyNone(t)
	tests := []struct {
		name        string
		stopTimeout time.Duration
	}{
		{
			name:        "in-flight request resolves after Stop",
			stopTimeout: 500 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := make(chan struct{})
			batchFn := func(_ context.Context, _ string, _ []string) (map[string]*types.GetShardOwnerResponse, map[string]struct{}, error) {
				<-block
				return nil, nil, nil
			}

			b := newShardBatcher(time.Second, batchFn)
			b.Start()

			errCh := make(chan error, 1)
			go func() {
				_, err := b.Submit(context.Background(), &types.GetShardOwnerRequest{Namespace: "ns", ShardKey: "shard-1"})
				errCh <- err
			}()

			// Give the request time to start flushing.
			time.Sleep(20 * time.Millisecond)

			// Unblock the batchFn and stop the batcher.
			close(block)
			b.Stop()

			select {
			case <-errCh:
			case <-time.After(tc.stopTimeout):
				t.Fatal("Submit did not return after Stop")
			}
		})
	}
}
