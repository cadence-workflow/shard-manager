// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package queue

import (
	"testing"

	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/common/persistence"
	"github.com/cadence-workflow/shard-manager/common/reconciliation/invariant"
	"github.com/cadence-workflow/shard-manager/service/history/config"
	"github.com/cadence-workflow/shard-manager/service/history/execution"
	"github.com/cadence-workflow/shard-manager/service/history/shard"
	"github.com/cadence-workflow/shard-manager/service/history/task"
	"github.com/cadence-workflow/shard-manager/service/history/workflowcache"
	"github.com/cadence-workflow/shard-manager/service/worker/archiver"
)

func TestTransferQueueFactory(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctrl := gomock.NewController(t)
	mockShard := shard.NewTestContext(
		t, ctrl, &persistence.ShardInfo{
			ShardID:          10,
			RangeID:          1,
			TransferAckLevel: 0,
		},
		config.NewForTest())
	defer mockShard.Finish(t)

	mockProcessor := task.NewMockProcessor(ctrl)
	mockArchiver := archiver.NewMockClient(ctrl)
	mockInvariant := invariant.NewMockInvariant(ctrl)
	mockWorkflowCache := workflowcache.NewMockWFCache(ctrl)

	f := NewTransferQueueFactory(mockProcessor, mockArchiver, mockWorkflowCache)

	processor := f.CreateQueue(mockShard, execution.NewCache(mockShard), mockInvariant)

	if processor == nil {
		t.Error("NewTransferQueueProcessor returned nil")
	}
}

func TestTimerQueueFactory(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctrl := gomock.NewController(t)
	mockShard := shard.NewTestContext(
		t, ctrl, &persistence.ShardInfo{
			ShardID:          10,
			RangeID:          1,
			TransferAckLevel: 0,
		},
		config.NewForTest())
	defer mockShard.Finish(t)

	mockProcessor := task.NewMockProcessor(ctrl)
	mockArchiver := archiver.NewMockClient(ctrl)
	mockInvariant := invariant.NewMockInvariant(ctrl)

	f := NewTimerQueueFactory(mockProcessor, mockArchiver)
	processor := f.CreateQueue(mockShard, execution.NewCache(mockShard), mockInvariant)

	if processor == nil {
		t.Error("NewTimerQueueProcessor returned nil")
	}
}
