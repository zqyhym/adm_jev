package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStaticHosting 验证静态目录能提供首页、资源文件，并且预览接口仍可用。
func TestStaticHosting(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<html>preview workbench</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "assets", "app.js"), []byte("console.log('local asset')"), 0600); err != nil {
		t.Fatal(err)
	}
	router, err := newHTTPHandler(false, "", directory)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path    string
		status  int
		content string
	}{
		{"/", http.StatusOK, "preview workbench"},
		{"/assets/app.js", http.StatusOK, "local asset"},
		{"/missing.js", http.StatusNotFound, "404"},
		{"/api/preview", http.StatusMethodNotAllowed, "只接受 POST"},
		{"/api/preview/", http.StatusNotFound, "404"},
		{"/api/unknown", http.StatusNotFound, "404"},
		{"/mraid.js", http.StatusOK, "window.mraid"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.content) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
		})
	}
	post := httptest.NewRecorder()
	router.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(`{"input":"<div>local preview</div>"}`)))
	if post.Code != http.StatusOK || !strings.Contains(post.Body.String(), `"format":"banner"`) {
		t.Fatalf("preview status=%d body=%s", post.Code, post.Body)
	}
}

// TestDevelopmentProxy 验证开发模式把页面请求代理到前端服务，且不代理本服务接口。
func TestDevelopmentProxy(t *testing.T) {
	// A cancellable context matches real server requests without legacy CloseNotify.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.RequestURI()
		if r.URL.Path == "/missing" {
			http.Error(w, "vite missing", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = fmt.Fprint(w, "vite: "+r.URL.RequestURI())
	}))
	defer upstream.Close()
	router, err := newHTTPHandler(true, upstream.URL, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/src/main.ts?version=1", "/missing"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil))
		wantStatus := http.StatusOK
		if path == "/missing" {
			wantStatus = http.StatusNotFound
		}
		if recorder.Code != wantStatus || !strings.Contains(recorder.Body.String(), "vite") {
			t.Fatalf("path=%s status=%d body=%s", path, recorder.Code, recorder.Body)
		}
		if got := <-requests; got != path {
			t.Fatalf("proxy path=%s; want %s", got, path)
		}
	}
	for _, path := range []string{"/api/preview", "/api/empty", "/mraid.js"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"input":"<div>local</div>"}`)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("path=%s status=%d body=%s", path, recorder.Code, recorder.Body)
		}
	}
	for _, path := range []string{"/api", "/api/unknown", "/api/preview/", "/api/empty/", "/api-extra", "/mraid.js/", "/mraid.js.map"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequestWithContext(ctx, method, path, nil))
			if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "404") {
				t.Fatalf("method=%s path=%s status=%d body=%s", method, path, recorder.Code, recorder.Body)
			}
		}
	}
	select {
	case request := <-requests:
		t.Fatalf("API was proxied: %s", request)
	default:
	}
}

// TestInvalidProxyAddress 验证非法代理地址无法创建服务。
func TestInvalidProxyAddress(t *testing.T) {
	if _, err := newHTTPHandler(true, "://invalid", t.TempDir()); err == nil {
		t.Fatal("expected invalid proxy address to fail")
	}
}
