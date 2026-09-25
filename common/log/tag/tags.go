// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tag

import (
	"time"
)

// Error returns tag for Error
func Error(err error) Tag {
	return newErrorTag("error", err)
}

// ClusterName returns tag for ClusterName
func ClusterName(clusterName string) Tag {
	return newStringTag("cluster-name", clusterName)
}

func workflowAction(action string) Tag {
	return newPredefinedStringTag("wf-action", action)
}

// WorkflowDomainName returns tag for WorkflowDomainName
func WorkflowDomainName(domainName string) Tag {
	return newStringTag("wf-domain-name", domainName)
}

// WorkflowScheduleID returns tag for WorkflowScheduleID
func WorkflowScheduleID(scheduleID int64) Tag {
	return newInt64("wf-schedule-id", scheduleID)
}

// component returns tag for component
func component(component string) Tag {
	return newPredefinedStringTag("component", component)
}

// clientOperation returns tag for clientOperation
func clientOperation(clientOperation string) Tag {
	return newPredefinedStringTag("client-operation", clientOperation)
}

// Service returns tag for Service
func Service(sv string) Tag {
	return newStringTag("service", sv)
}

// Address return tag for Address
func Address(ad string) Tag {
	return newStringTag("address", ad)
}

// Key returns tag for Key
func Key(k string) Tag {
	return newStringTag("key", k)
}

// Value returns tag for Value
func Value(v interface{}) Tag {
	return newObjectTag("value", v)
}

// Port returns tag for Port
func Port(p int) Tag {
	return newInt("port", p)
}

// ClientError returns tag for ClientError
func ClientError(clientErr error) Tag {
	return newErrorTag("client-error", clientErr)
}

// Bool returns tag for Bool
func Bool(b bool) Tag {
	return newBoolTag("bool", b)
}

// ActorID returns tag for the actor ID
func ActorID(actorID string) Tag {
	return newStringTag("actor-id", actorID)
}

// LoggingCallAtKey is reserved tag
const LoggingCallAtKey = "logging-call-at"

// SysStackTrace returns tag for SysStackTrace
func SysStackTrace(stackTrace string) Tag {
	return newStringTag("sys-stack-trace", stackTrace)
}

// Dynamic Uses reflection based logging for arbitrary values
// for not very performant logging
func Dynamic(key string, v interface{}) Tag {
	return newPredefinedDynamicTag(key, v)
}

func ShardNamespace(name string) Tag {
	return newStringTag("shard-namespace", name)
}

func ShardExecutor(ID string) Tag {
	return newStringTag("shard-executor", ID)
}

func ShardExecutors(executorIDs []string) Tag {
	return newStringsTag("shard-executors", executorIDs)
}

func ShardKey(shardKey string) Tag {
	return newStringTag("shard-key", shardKey)
}

func ShardLoad(load string) Tag {
	return newStringTag("shard-load", load)
}

func ElectionDelay(t time.Duration) Tag {
	return newDurationTag("election-delay", t)
}
