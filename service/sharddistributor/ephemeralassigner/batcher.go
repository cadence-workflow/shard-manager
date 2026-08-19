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

	"github.com/cadence-workflow/shard-manager/common/types"
)

const (
	_requestsChannelLimit = 1024
)

// ephemeralAssignmentBatchFn assigns a batch of shards within a namespace.
// results maps each successfully assigned shard key to its owner.
// drained is the set of requested keys that must not be assigned; the batcher
// returns ShardDrainedError for those keys. Any other key absent from both maps
// is treated as an internal error.
type ephemeralAssignmentBatchFn func(ctx context.Context, namespace string, shardKeys []string) (results map[string]*types.GetShardOwnerResponse, drained map[string]struct{}, err error)

// batchRequest is a single caller's request submitted to the shardBatcher.
type batchRequest struct {
	namespace string
	shardKey  string
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
	namespace string
	results   map[string]*types.GetShardOwnerResponse
	drained   map[string]struct{}
	err       error
}

// namespaceState tracks inflight and pending requests for a single namespace.
// Only accessed from the loop goroutine — no synchronization needed.
type namespaceState struct {
	inflight []*batchRequest
	pending  []*batchRequest
}

// shardBatcher coalesces GetShardOwner calls for ephemeral namespaces. The first
// request for a namespace triggers an immediate flush. Requests that arrive while
// a flush is in-flight accumulate and are processed as the next batch as soon as
// the current one completes. At most one flush per namespace is in-flight at any
// time — etcd write latency acts as the natural batching window.
//
// Usage:
//
//	b := newShardBatcher(5*time.Second, processFn)
//	b.Start()
//	defer b.Stop()
//	resp, err := b.Submit(ctx, &types.GetShardOwnerRequest{Namespace: namespace, ShardKey: shardKey})
type shardBatcher struct {
	timeout      time.Duration
	processBatch ephemeralAssignmentBatchFn

	requestChan chan *batchRequest

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newShardBatcher(timeout time.Duration, processBatch ephemeralAssignmentBatchFn) *shardBatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &shardBatcher{
		timeout:      timeout,
		processBatch: processBatch,
		requestChan:  make(chan *batchRequest, _requestsChannelLimit),
		ctx:          ctx,
		cancel:       cancel,
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

// Submit enqueues a single GetShardOwner request and blocks until the batcher
// has processed the batch that contains it, or until ctx is cancelled.
func (b *shardBatcher) Submit(ctx context.Context, request *types.GetShardOwnerRequest) (*types.GetShardOwnerResponse, error) {
	req := &batchRequest{
		namespace: request.Namespace,
		shardKey:  request.ShardKey,
		respChan:  make(chan batchResponse, 1),
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

func (b *shardBatcher) loop() {
	defer b.wg.Done()

	namespaces := make(map[string]*namespaceState)
	flushDone := make(chan flushResult, _requestsChannelLimit)

	for {
		select {
		case req := <-b.requestChan:
			ns := namespaces[req.namespace]
			if ns == nil {
				ns = &namespaceState{}
				namespaces[req.namespace] = ns
			}

			if ns.inflight == nil {
				ns.inflight = []*batchRequest{req}
				b.startFlush(req.namespace, ns.inflight, flushDone)
			} else {
				ns.pending = append(ns.pending, req)
			}

		case res := <-flushDone:
			ns := namespaces[res.namespace]
			b.deliverResults(ns.inflight, res.results, res.drained, res.err)

			if len(ns.pending) > 0 {
				ns.inflight = ns.pending
				ns.pending = nil
				b.startFlush(res.namespace, ns.inflight, flushDone)
			} else {
				delete(namespaces, res.namespace)
			}

		case <-b.ctx.Done():
			b.drainAndCancel(namespaces, flushDone)
			return
		}
	}
}

// startFlush spawns a goroutine that calls processBatch and sends the result
// back on done.
func (b *shardBatcher) startFlush(namespace string, reqs []*batchRequest, done chan<- flushResult) {
	shardKeys := make([]string, len(reqs))
	for i, r := range reqs {
		shardKeys[i] = r.shardKey
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
		defer cancel()

		results, drained, err := b.processBatch(ctx, namespace, shardKeys)
		done <- flushResult{namespace: namespace, results: results, drained: drained, err: err}
	}()
}

// deliverResults writes the batch outcome to every caller's respChan.
func (b *shardBatcher) deliverResults(reqs []*batchRequest, results map[string]*types.GetShardOwnerResponse, drained map[string]struct{}, err error) {
	for _, req := range reqs {
		// Non-blocking write: respChan has capacity 1 and each req has
		// exactly one writer (this loop) and one reader (Submit).
		req.respChan <- toBatchResponse(results, drained, err, req.namespace, req.shardKey)
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

func (b *shardBatcher) drainAndCancel(namespaces map[string]*namespaceState, flushDone chan flushResult) {
	// Wait for any in-flight flushes to complete and deliver their results.
	inflightCount := 0
	for _, ns := range namespaces {
		if ns.inflight != nil {
			inflightCount++
		}
	}
	for range inflightCount {
		res := <-flushDone
		ns := namespaces[res.namespace]
		b.deliverResults(ns.inflight, res.results, res.drained, res.err)
	}

	// Cancel all pending requests that never got flushed.
	for _, ns := range namespaces {
		for _, req := range ns.pending {
			req.respChan <- batchResponse{err: b.ctx.Err()}
		}
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
