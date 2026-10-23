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
	"sync"
	"time"

	"github.com/cadence-workflow/shard-manager/common/clock"
	"github.com/cadence-workflow/shard-manager/common/types"
)

const (
	_requestsChannelLimit = 1024
)

// ephemeralAssignmentBatchFn assigns a batch of shards of the batcher's namespace.
// results maps each successfully assigned shard key to its owner.
// drained is the set of requested keys that must not be assigned; the batcher
// returns ShardDrainedError for those keys. Any other key absent from both maps
// is treated as an internal error.
type ephemeralAssignmentBatchFn func(ctx context.Context, shardKeys []string) (results map[string]*types.GetShardOwnerResponse, drained map[string]struct{}, err error)

// batchRequest is a single caller's request submitted to the shardBatcher.
type batchRequest struct {
	shardKey string
	// respChan is a buffered channel (cap 1) so the batcher goroutine never blocks
	// when writing the result back.
	respChan chan batchResponse
}

type batchResponse struct {
	resp *types.GetShardOwnerResponse
	err  error
}

// flushResult is sent back from a flush goroutine to the event loop.
type flushResult struct {
	results map[string]*types.GetShardOwnerResponse
	drained map[string]struct{}
	err     error
}

// shardBatcher coalesces GetShardOwner calls for one ephemeral namespace. The
// first request while idle starts a short collection window. Requests that
// arrive while a flush is in-flight accumulate and are processed as the next
// batch as soon as the current one completes. At most one flush is in-flight at
// any time.
//
// Usage:
//
//	b := newShardBatcher(namespace, timeSource, 5*time.Second, coalescingWindow, processFn)
//	b.Start()
//	defer b.Stop()
//	resp, err := b.Submit(ctx, shardKey)
type shardBatcher struct {
	namespace        string
	timeSource       clock.TimeSource
	timeout          time.Duration
	coalescingWindow func(namespace string) time.Duration
	processBatch     ephemeralAssignmentBatchFn

	requestChan chan *batchRequest

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newShardBatcher(namespace string, timeSource clock.TimeSource, timeout time.Duration, coalescingWindow func(namespace string) time.Duration, processBatch ephemeralAssignmentBatchFn) *shardBatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &shardBatcher{
		namespace:        namespace,
		timeSource:       timeSource,
		timeout:          timeout,
		coalescingWindow: coalescingWindow,
		processBatch:     processBatch,
		requestChan:      make(chan *batchRequest, _requestsChannelLimit),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start launches the background coalescing loop.
func (b *shardBatcher) Start() {
	b.wg.Add(1)
	go b.loop()
}

// Stop signals the coalescing loop to shut down and waits for it to finish.
// Any requests that have already been submitted but not yet flushed will be
// drained and cancelled before the loop exits.
func (b *shardBatcher) Stop() {
	b.cancel()
	b.wg.Wait()
}

// Submit enqueues a single shard key and blocks until the batcher has processed
// the batch that contains it, or until ctx is cancelled.
func (b *shardBatcher) Submit(ctx context.Context, shardKey string) (*types.GetShardOwnerResponse, error) {
	req := &batchRequest{
		shardKey: shardKey,
		respChan: make(chan batchResponse, 1),
	}

	select {
	case b.requestChan <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.ctx.Done():
		return nil, b.ctx.Err()
	}

	select {
	case res := <-req.respChan:
		return res.resp, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.ctx.Done():
		return nil, b.ctx.Err()
	}
}

// loop owns all batching state, so none of it needs synchronization.
// inflight is nil exactly when no flush is running.
func (b *shardBatcher) loop() {
	defer b.wg.Done()

	var inflight, pending []*batchRequest
	scheduled := false
	flushDone := make(chan flushResult, 1)
	flushReady := make(chan struct{}, 1)

	for {
		select {
		case req := <-b.requestChan:
			pending = append(pending, req)
			if inflight == nil && !scheduled {
				scheduled = true
				b.scheduleFlush(flushReady)
			}

		case <-flushReady:
			scheduled = false
			inflight, pending = pending, nil
			b.startFlush(inflight, flushDone)

		case res := <-flushDone:
			b.deliverResults(inflight, res)
			inflight = nil
			if len(pending) > 0 {
				inflight, pending = pending, nil
				b.startFlush(inflight, flushDone)
			}

		case <-b.ctx.Done():
			b.drainAndCancel(inflight, pending, flushDone)
			return
		}
	}
}

func (b *shardBatcher) scheduleFlush(ready chan<- struct{}) {
	go func() {
		timer := b.timeSource.NewTimer(b.coalescingWindow(b.namespace))
		defer timer.Stop()
		select {
		case <-timer.Chan():
			select {
			case ready <- struct{}{}:
			case <-b.ctx.Done():
			}
		case <-b.ctx.Done():
		}
	}()
}

// startFlush spawns a goroutine that calls processBatch and sends the result
// back on done.
func (b *shardBatcher) startFlush(reqs []*batchRequest, done chan<- flushResult) {
	shardKeys := make([]string, len(reqs))
	for i, r := range reqs {
		shardKeys[i] = r.shardKey
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()

		results, drained, err := b.processBatch(ctx, shardKeys)
		done <- flushResult{results: results, drained: drained, err: err}
	}()
}

// deliverResults writes the batch outcome to every caller's respChan.
func (b *shardBatcher) deliverResults(reqs []*batchRequest, res flushResult) {
	for _, req := range reqs {
		// Non-blocking write: respChan has capacity 1 and each req has
		// exactly one writer (this loop) and one reader (Submit).
		req.respChan <- toBatchResponse(res.results, res.drained, res.err, b.namespace, req.shardKey)
	}
}

// toBatchResponse picks one caller's outcome out of a whole batch's result.
func toBatchResponse(results map[string]*types.GetShardOwnerResponse, drained map[string]struct{}, err error, namespace, shardKey string) batchResponse {
	if err != nil {
		return batchResponse{err: err}
	}
	if _, isDrained := drained[shardKey]; isDrained {
		return batchResponse{err: &types.ShardDrainedError{
			Namespace: namespace,
			ShardKey:  shardKey,
		}}
	}
	resp := results[shardKey]
	if resp == nil {
		// processBatch is expected to always include an entry for
		// every key it was given; a missing entry is an internal error.
		return batchResponse{err: &types.InternalServiceError{
			Message: "batch processor returned no result for shard key: " + shardKey,
		}}
	}
	return batchResponse{resp: resp}
}

func (b *shardBatcher) drainAndCancel(inflight, pending []*batchRequest, flushDone chan flushResult) {
	// Wait for the in-flight flush to complete and deliver its results.
	if inflight != nil {
		b.deliverResults(inflight, <-flushDone)
	}

	// Cancel the pending requests that never got flushed.
	for _, req := range pending {
		req.respChan <- batchResponse{err: b.ctx.Err()}
	}

	// Drain any remaining requests from the channel.
	for {
		select {
		case req := <-b.requestChan:
			req.respChan <- batchResponse{err: b.ctx.Err()}
		default:
			return
		}
	}
}
