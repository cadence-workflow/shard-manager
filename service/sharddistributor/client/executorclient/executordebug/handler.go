// Package executordebug renders a debug page describing the shard distributor
// executors running in the current process.
package executordebug

import (
	"net/http"

	"go.uber.org/fx"
)

const (
	// RouteName and RoutePattern are the page's identity when it is mounted on
	// a debug mux.
	RouteName    = "Shard Distributor Executor"
	RoutePattern = "/debug/shard-distributor-executor/"
)

var _page = []byte("# shard-distributor executor\n")

// Handler renders the shard distributor executor debug page.
type Handler struct{}

// NewHandler builds the debug page handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Markdown renders the page as markdown. The signature matches the Markdowner
// interface of Uber's debug page framework, so that framework can mount this
// handler without this package having to depend on it.
func (h *Handler) Markdown(_ *http.Request) ([]byte, error) {
	return _page, nil
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

// Module provides the debug page handler.
var Module = fx.Module("shard-distributor-executor-debug",
	fx.Provide(NewHandler),
)
