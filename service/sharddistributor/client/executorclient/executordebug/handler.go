// Package executordebug renders a debug page describing the shard distributor
// executors running in the current process.
package executordebug

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"text/template"

	"go.uber.org/fx"

	"github.com/cadence-workflow/shard-manager/service/sharddistributor/client/executorclient"

	_ "embed" // required for embedding the page template
)

const (
	// RouteName and RoutePattern are the page's identity when it is mounted on
	// a debug mux.
	RouteName    = "Shard Distributor Executor"
	RoutePattern = "/debug/shard-distributor-executor/"
)

//go:embed debug.tmpl
var _debugTemplate string

var _tmpl = template.Must(template.New("shard-distributor-executor-debug").Parse(_debugTemplate))

// Handler renders the shard distributor executor debug page.
type Handler struct {
	executors []executorclient.ExecutorInfo
}

type page struct {
	Executors []executorView
}

type executorView struct {
	Namespace  string
	ExecutorID string
	Metadata   map[string]string
}

// NewHandler builds the debug page over every executor registered in the process.
func NewHandler(executors executorclient.ExecutorInfos) *Handler {
	return &Handler{executors: executors.Infos}
}

// Markdown renders the page as markdown. The signature matches the Markdowner
// interface of Uber's debug page framework, so that framework can mount this
// handler without this package having to depend on it.
func (h *Handler) Markdown(_ *http.Request) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := _tmpl.Execute(buf, page{Executors: h.executorViews()}); err != nil {
		return nil, fmt.Errorf("render shard distributor executor debug page: %w", err)
	}
	return buf.Bytes(), nil
}

// ServeHTTP serves the raw markdown, for use outside a debug page framework.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	markdown, err := h.Markdown(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(markdown)
}

// executorViews sorts the executors so the page is stable across reloads: the
// value group they arrive in has no defined order.
func (h *Handler) executorViews() []executorView {
	views := make([]executorView, 0, len(h.executors))
	for _, executor := range h.executors {
		views = append(views, executorView{
			Namespace:  executor.GetNamespace(),
			ExecutorID: executor.GetExecutorID(),
			Metadata:   executor.GetMetadata(),
		})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Namespace != views[j].Namespace {
			return views[i].Namespace < views[j].Namespace
		}
		return views[i].ExecutorID < views[j].ExecutorID
	})
	return views
}

// Module provides the debug page handler.
var Module = fx.Module("shard-distributor-executor-debug",
	fx.Provide(NewHandler),
)
