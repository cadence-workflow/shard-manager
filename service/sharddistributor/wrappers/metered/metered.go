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

package metered

import (
	"context"
	"errors"

	"go.uber.org/yarpc/yarpcerrors"

	"github.com/cadence-workflow/shard-manager/common/log"
	"github.com/cadence-workflow/shard-manager/common/log/tag"
	"github.com/cadence-workflow/shard-manager/common/metrics"
	"github.com/cadence-workflow/shard-manager/common/types"
)

func handleErr(err error, scope metrics.Scope, logger log.Logger) error {
	logger = logger.Helper()
	if classifyErr(err, scope, logger) {
		return err
	}
	logger.Error("internal uncategorized error", tag.Error(err))
	scope.IncCounter(metrics.ShardDistributorFailures)
	return err
}

// handleStreamErr classifies errors from long-lived streaming RPCs, where cancellation is the normal
// exit on client disconnect and server shutdown rather than a failure. Unary RPCs keep reporting a
// cancellation as a failure, since there it is not part of the expected lifecycle.
func handleStreamErr(err error, scope metrics.Scope, logger log.Logger) error {
	logger = logger.Helper()
	if classifyErr(err, scope, logger) {
		return err
	}
	if isCanceled(err) {
		logger.Debug("stream canceled", tag.Error(err))
		scope.IncCounter(metrics.ShardDistributorErrContextCanceledCounter)
		return err
	}
	logger.Error("internal uncategorized error", tag.Error(err))
	scope.IncCounter(metrics.ShardDistributorFailures)
	return err
}

// classifyErr emits the metric and log for every error shape both handlers agree on, and reports
// whether err was one of them.
func classifyErr(err error, scope metrics.Scope, logger log.Logger) bool {
	switch {
	case errors.As(err, new(*types.InternalServiceError)):
		scope.IncCounter(metrics.ShardDistributorFailures)
		logger.Error("Internal service error", tag.Error(err))
	case errors.As(err, new(*types.NamespaceNotFoundError)):
		scope.IncCounter(metrics.ShardDistributorErrNamespaceNotFound)
	case errors.As(err, new(*types.ShardNotFoundError)):
		scope.IncCounter(metrics.ShardDistributorErrShardNotFound)
	case errors.Is(err, context.DeadlineExceeded):
		logger.Error("request timeout", tag.Error(err))
		scope.IncCounter(metrics.ShardDistributorErrContextTimeoutCounter)
	default:
		return false
	}
	return true
}

func isCanceled(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	// A peer going away mid-stream surfaces as a YARPC code, not a wrapped context.Canceled.
	return yarpcerrors.FromError(err).Code() == yarpcerrors.CodeCancelled
}
