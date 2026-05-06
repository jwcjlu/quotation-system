package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	klog "github.com/go-kratos/kratos/v2/log"
)

// scriptStoreFileHandler 将 urlPrefix 下的 GET/HEAD 映射到 root 子路径；防目录穿越。
func scriptStoreFileHandler(root, urlPrefix string) http.Handler {
	root = filepath.Clean(root)
	pref := "/" + strings.Trim(strings.TrimSpace(urlPrefix), "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		klog.Infof("script_store: request method=%s path=%s prefix=%s root=%s", r.Method, r.URL.Path, pref, root)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			klog.Warnf("script_store: reject method method=%s path=%s", r.Method, r.URL.Path)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		upath := r.URL.Path
		if !strings.HasPrefix(upath, pref+"/") && upath != pref {
			klog.Warnf("script_store: reject prefix mismatch path=%s prefix=%s", upath, pref)
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(upath, pref)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			klog.Warnf("script_store: reject empty relative path path=%s prefix=%s", upath, pref)
			http.NotFound(w, r)
			return
		}
		for _, p := range strings.Split(filepath.ToSlash(rel), "/") {
			if p == ".." || p == "" {
				klog.Warnf("script_store: reject unsafe segment rel=%s segment=%s", rel, p)
				http.NotFound(w, r)
				return
			}
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			klog.Errorf("script_store: abs root error root=%s err=%v", root, err)
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		fp := filepath.Join(absRoot, filepath.FromSlash(rel))
		absFP, err := filepath.Abs(fp)
		if err != nil {
			klog.Warnf("script_store: abs file path error file=%s err=%v", fp, err)
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(absFP, absRoot+string(os.PathSeparator)) && absFP != absRoot {
			klog.Warnf("script_store: reject path escape rel=%s abs_root=%s abs_file=%s", rel, absRoot, absFP)
			http.NotFound(w, r)
			return
		}
		klog.Infof("script_store: serve file rel=%s abs_root=%s abs_file=%s", rel, absRoot, absFP)
		http.ServeFile(w, r, absFP)
	})
}
