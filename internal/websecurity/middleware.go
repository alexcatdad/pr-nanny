// Package websecurity constrains the embedded renderer and Wails runtime bridge.
package websecurity

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

const CSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'; frame-ancestors 'none'"

// Middleware prevents access to Wails' generic browser, dialog, clipboard,
// window-navigation and system APIs. The app exposes only its bound service.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", CSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/wails/runtime" {
			if r.Header.Get("x-wails-chunk-id") != "" {
				http.Error(w, "Unsupported request", http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
			if err != nil {
				http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
				return
			}
			var req struct {
				Object *int `json:"object"`
				Method *int `json:"method"`
			}
			if len(data) > 0 {
				if json.Unmarshal(data, &req) != nil {
					http.Error(w, "Invalid request", http.StatusBadRequest)
					return
				}
			} else {
				if n, err := strconv.Atoi(r.URL.Query().Get("object")); err == nil {
					req.Object = &n
				}
				if n, err := strconv.Atoi(r.URL.Query().Get("method")); err == nil {
					req.Method = &n
				}
			}
			if req.Object == nil || req.Method == nil || !allowed(*req.Object, *req.Method) {
				http.Error(w, "Unavailable capability", http.StatusForbidden)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(data))
		}
		next.ServeHTTP(w, r)
	})
}
func allowed(object, method int) bool {
	return (object == 0 && method == 0) || object == 10 || object == 3
}
