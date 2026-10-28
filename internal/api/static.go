package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) staticHandler() http.Handler {
	root := filepath.Clean(s.cfg.WebDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if _, err := s.currentUser(r); err != nil {
				http.Redirect(w, r, "/login.html", http.StatusFound)
				return
			}
		}
		clean := filepath.Clean("/" + r.URL.Path)
		rel := strings.TrimPrefix(clean, string(filepath.Separator))
		file := filepath.Join(root, filepath.FromSlash(rel))
		if !strings.HasPrefix(file, root+string(filepath.Separator)) && file != root {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(file)
		if err == nil && info.IsDir() {
			// A directory route (notably /vue/) serves its own index before
			// the legacy SPA fallback.
			candidate := filepath.Join(file, "index.html")
			if cinfo, cerr := os.Stat(candidate); cerr == nil && !cinfo.IsDir() {
				file = candidate
			} else {
				file = filepath.Join(root, "index.html")
			}
		} else if err != nil {
			// SPA fallback; do not turn missing assets into HTML.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
				return
			}
			file = filepath.Join(root, "index.html")
		}
		switch strings.ToLower(filepath.Ext(file)) {
		case ".mjs":
			// Windows 注册表里通常没有 .mjs 的 MIME，ServeFile 会退成 text/plain，
			// 浏览器按规范拒绝把它当 ES module 执行（pdf.js 等 vendor 模块依赖这一点）。
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case ".wasm":
			w.Header().Set("Content-Type", "application/wasm")
		}
		http.ServeFile(w, r, file)
	})
}
