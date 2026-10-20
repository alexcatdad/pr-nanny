package websecurity

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCapabilityBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body, query string
		status            int
	}{
		{"binding", `{"object":0,"method":0,"args":{"args":["synthetic-secret"]}}`, "", 200},
		{"cancel", `{"object":10,"method":0}`, "", 200},
		{"events-emit", `{"object":3,"method":0}`, "", 403},
		{"events-other-method", `{"object":3,"method":1}`, "", 403},
		{"binding-other-method", `{"object":0,"method":1}`, "", 403},
		{"cancel-other-method", `{"object":10,"method":1}`, "", 403},
		{"browser", `{"object":9,"method":0}`, "", 403},
		{"dialog", `{"object":5,"method":0}`, "", 403},
		{"filesystem", `{"object":8,"method":0}`, "", 403},
		{"navigate", `{"object":6,"method":14}`, "", 403},
		{"missing", "{}", "", 403},
		{"malformed", "{", "", 400},
		{"query-browser", "", "?object=9&method=0", 403},
		{"query-binding", "", "?object=0&method=0", 200},
		{"body-precedence", `{"object":9,"method":0}`, "?object=0&method=0", 403},
		{"oversize", strings.Repeat("x", 17<<10), "", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				if !bytes.Equal(data, []byte(tc.body)) {
					t.Error("body changed")
				}
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/wails/runtime"+tc.query, strings.NewReader(tc.body))
			h.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Errorf("status %d want %d", rec.Code, tc.status)
			}
			if strings.Contains(rec.Body.String(), "synthetic-secret") {
				t.Error("request leaked in response")
			}
			if rec.Header().Get("Content-Security-Policy") != CSP {
				t.Error("missing CSP")
			}
		})
	}
}
func TestChunkedRequestsRejected(t *testing.T) {
	req := httptest.NewRequest("POST", "/wails/runtime", strings.NewReader(`{"object":0,"method":0}`))
	req.Header.Set("x-wails-chunk-id", "x")
	rec := httptest.NewRecorder()
	Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("forwarded chunk") })).ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
}
func TestAssetsRetainSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello")) })).ServeHTTP(rec, httptest.NewRequest("GET", "/index.html", nil))
	if rec.Code != 200 || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing header")
	}
}
