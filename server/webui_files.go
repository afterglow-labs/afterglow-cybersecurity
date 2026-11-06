package server

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// Keep compression confined to public files, outside authenticated APIs and SSE.
func staticWebUI(root fs.FS) http.Handler {
	var tags sync.Map
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			target := "/function/tasks"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		name, found := strings.TrimPrefix(path.Clean(r.URL.Path), "/"), ""
		for _, candidate := range []string{name, name + "/index.html", name + ".html"} {
			if info, err := fs.Stat(root, candidate); err == nil && !info.IsDir() {
				found = candidate
				break
			}
		}
		if found == "" {
			if strings.HasPrefix(name, "_next/") {
				http.NotFound(w, r)
				return
			}
			found = "index.html"
		}
		selected, encoding, best := found, "", encodingQuality(r.Header.Get("Accept-Encoding"), "identity")
		for _, option := range []struct{ suffix, encoding string }{{".br", "br"}, {".gz", "gzip"}} {
			quality := encodingQuality(r.Header.Get("Accept-Encoding"), option.encoding)
			if quality <= 0 || quality < best || (quality == best && encoding != "") {
				continue
			}
			if info, err := fs.Stat(root, found+option.suffix); err == nil && !info.IsDir() {
				selected, encoding, best = found+option.suffix, option.encoding, quality
			}
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if encoding == "" && best == 0 {
			http.Error(w, "No acceptable asset encoding", http.StatusNotAcceptable)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		if contentType := mime.TypeByExtension(path.Ext(found)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if strings.HasPrefix(found, "_next/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if value, ok := tags.Load(selected); ok {
			w.Header().Set("ETag", value.(string))
		} else if data, err := fs.ReadFile(root, selected); err == nil {
			tag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
			tags.Store(selected, tag)
			w.Header().Set("ETag", tag)
		}
		http.ServeFileFS(w, r, root, selected)
	})
}

func encodingQuality(header, encoding string) float64 {
	quality, wildcard := -1.0, -1.0
	for _, entry := range strings.Split(header, ",") {
		parts := strings.Split(entry, ";")
		name, weight := strings.ToLower(strings.TrimSpace(parts[0])), 1.0
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(key, "q") {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil || !(parsed >= 0 && parsed <= 1) {
					weight = 0
				} else {
					weight = parsed
				}
			}
		}
		if name == encoding {
			quality = weight
		}
		if name == "*" {
			wildcard = weight
		}
	}
	if quality >= 0 {
		return quality
	}
	if wildcard >= 0 {
		return wildcard
	}
	if encoding == "identity" {
		return 1
	}
	return 0
}
