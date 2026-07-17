package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type ready bool

func (r ready) Ready() bool { return bool(r) }
func TestEndpoints(t *testing.T) {
	for _, tc := range []struct {
		r    ready
		path string
		want int
	}{{false, "/healthz", 200}, {false, "/readyz", 503}, {true, "/readyz", 200}} {
		s := New(3000, tc.r)
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		s.s.Handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
}
