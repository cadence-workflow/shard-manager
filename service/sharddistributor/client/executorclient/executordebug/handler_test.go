package executordebug

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestMarkdown(t *testing.T) {
	markdown, err := NewHandler().Markdown(httptest.NewRequest(http.MethodGet, RoutePattern, nil))

	require.NoError(t, err)
	assert.Equal(t, "# shard-distributor executor\n", string(markdown))
}

func TestServeHTTP(t *testing.T) {
	recorder := httptest.NewRecorder()

	NewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, RoutePattern, nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "# shard-distributor executor\n", recorder.Body.String())
}

func TestModule(t *testing.T) {
	var handler *Handler

	fxtest.New(t,
		Module,
		fx.Populate(&handler),
	).RequireStart().RequireStop()

	require.NotNil(t, handler)
}
