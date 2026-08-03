package executordebug

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"go.uber.org/mock/gomock"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"
)

func newExecutorInfo(ctrl *gomock.Controller, namespace, executorID string, metadata map[string]string) executorclient.ExecutorInfo {
	info := executorclient.NewMockExecutorInfo(ctrl)
	info.EXPECT().GetNamespace().Return(namespace).AnyTimes()
	info.EXPECT().GetExecutorID().Return(executorID).AnyTimes()
	info.EXPECT().GetMetadata().Return(metadata).AnyTimes()
	return info
}

func TestMarkdown(t *testing.T) {
	ctrl := gomock.NewController(t)

	tests := []struct {
		name      string
		executors []executorclient.ExecutorInfo
		expected  string
	}{
		{
			name:      "no executors",
			executors: nil,
			expected: `# Shard Distributor Executors

No shard distributor executors are registered in this process.
`,
		},
		{
			name: "executor without metadata",
			executors: []executorclient.ExecutorInfo{
				newExecutorInfo(ctrl, "test-namespace", "executor-1", nil),
			},
			expected: `# Shard Distributor Executors

## test-namespace

Executor ID: ` + "`executor-1`" + `

This executor reports no metadata.
`,
		},
		{
			name: "executors are sorted by namespace then ID",
			executors: []executorclient.ExecutorInfo{
				newExecutorInfo(ctrl, "namespace-b", "executor-1", nil),
				newExecutorInfo(ctrl, "namespace-a", "executor-2", map[string]string{"grpc-address": "10.0.0.1:1234"}),
				newExecutorInfo(ctrl, "namespace-a", "executor-1", nil),
			},
			expected: `# Shard Distributor Executors

## namespace-a

Executor ID: ` + "`executor-1`" + `

This executor reports no metadata.

## namespace-a

Executor ID: ` + "`executor-2`" + `

| Metadata | Value |
| :---     |  ---: |
| grpc-address | 10.0.0.1:1234 |

## namespace-b

Executor ID: ` + "`executor-1`" + `

This executor reports no metadata.
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(executorclient.ExecutorInfos{Infos: tt.executors})

			markdown, err := handler.Markdown(httptest.NewRequest(http.MethodGet, RoutePattern, nil))

			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(markdown))
		})
	}
}

func TestServeHTTP(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler := NewHandler(executorclient.ExecutorInfos{
		Infos: []executorclient.ExecutorInfo{
			newExecutorInfo(ctrl, "test-namespace", "executor-1", nil),
		},
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, RoutePattern, nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Body.String(), "test-namespace")
	assert.Contains(t, recorder.Body.String(), "executor-1")
}

func TestModule(t *testing.T) {
	ctrl := gomock.NewController(t)

	var handler *Handler
	fxtest.New(t,
		fx.Provide(func() executorclient.ExecutorInfoResult {
			return executorclient.ExecutorInfoResult{
				Info: newExecutorInfo(ctrl, "test-namespace", "executor-1", nil),
			}
		}),
		Module,
		fx.Populate(&handler),
	).RequireStart().RequireStop()

	markdown, err := handler.Markdown(httptest.NewRequest(http.MethodGet, RoutePattern, nil))

	require.NoError(t, err)
	assert.Contains(t, string(markdown), "executor-1")
}
