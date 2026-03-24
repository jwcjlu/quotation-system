package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// scriptStoreFileHandler 将 urlPrefix 下的 GET/HEAD 映射到 root 子路径；防目录穿越。
func scriptStoreFileHandler(root, urlPrefix string) http.Handler {
	root = filepath.Clean(root)
	pref := "/" + strings.Trim(strings.TrimSpace(urlPrefix), "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		upath := r.URL.Path
		if !strings.HasPrefix(upath, pref+"/") && upath != pref {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(upath, pref)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			http.NotFound(w, r)
			return
		}
		for _, p := range strings.Split(filepath.ToSlash(rel), "/") {
			if p == ".." || p == "" {
				http.NotFound(w, r)
				return
			}
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		fp := filepath.Join(absRoot, filepath.FromSlash(rel))
		absFP, err := filepath.Abs(fp)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(absFP, absRoot+string(os.PathSeparator)) && absFP != absRoot {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, absFP)
	})
}
