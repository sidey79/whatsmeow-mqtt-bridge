package health

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Readiness interface{ Ready() bool }
type Server struct{ s *http.Server }

func New(port int, r Readiness) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !r.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	return &Server{s: &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
}
func (s *Server) ListenAndServe() error              { return s.s.ListenAndServe() }
func (s *Server) Shutdown(ctx context.Context) error { return s.s.Shutdown(ctx) }
