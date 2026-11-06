package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticAssetEncodingAndRevalidation(t *testing.T) {
	raw := []byte(strings.Repeat("window.aptex = 'ready';\n", 100))
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write(raw)
	_ = writer.Close()
	root := fstest.MapFS{
		"index.html":                     {Data: []byte("fallback")},
		"function/tasks/index.html":      {Data: []byte("tasks")},
		"_next/static/chunks/hash.js":    {Data: raw},
		"_next/static/chunks/hash.js.gz": {Data: compressed.Bytes()},
		"_next/static/chunks/hash.js.br": {Data: []byte("prebuilt Brotli bytes")},
	}
	handler := staticWebUI(root)
	for _, tc := range []struct{ accept, encoding string }{
		{"", ""}, {"gzip", "gzip"}, {"br,gzip", "br"},
		{"br;q=0,gzip", "gzip"}, {"br;q=0.1,gzip;q=1", "gzip"},
		{"gzip;q=0,br;q=0", ""}, {"br;q=NaN,gzip", "gzip"},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/_next/static/chunks/hash.js", nil)
			req.Header.Set("Accept-Encoding", tc.accept)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != 200 || response.Header().Get("Content-Encoding") != tc.encoding {
				t.Fatalf("response: %d %v", response.Code, response.Header())
			}
			if !strings.Contains(response.Header().Get("Content-Type"), "javascript") || !strings.Contains(response.Header().Get("Cache-Control"), "immutable") || response.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatal(response.Header())
			}
			if tc.encoding == "gzip" {
				reader, err := gzip.NewReader(response.Body)
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				decoded, err := io.ReadAll(reader)
				if err != nil || !bytes.Equal(decoded, raw) {
					t.Fatal("gzip payload changed", err)
				}
			}
			req.Header.Set("If-None-Match", response.Header().Get("ETag"))
			cached := httptest.NewRecorder()
			handler.ServeHTTP(cached, req)
			if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
				t.Fatal("unchanged asset was retransmitted", cached.Code)
			}
		})
	}
	for _, target := range []string{"/function/tasks", "/_next/static/missing.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("HTML or missing assets were cached permanently")
		}
		if strings.Contains(target, "missing") && response.Code != 404 {
			t.Fatal("missing script became HTML")
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/?lang=en", nil))
	if response.Code != 307 || response.Header().Get("Location") != "/function/tasks?lang=en" {
		t.Fatal("root must redirect without a JavaScript bootstrap", response.Code, response.Header())
	}
}
