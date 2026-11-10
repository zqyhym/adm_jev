package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/asset"
	"adm_jev/internal/creative/handle"
	"adm_jev/internal/creative/service"
	"adm_jev/internal/jev"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dev := flag.Bool("dev", false, "把页面请求代理到 Vite")
	vite := flag.String("vite", "http://127.0.0.1:5173", "Vite 地址")
	flag.Parse()

	staticDirectory := filepath.Join("..", "web", "dist")
	if *dev {
		log.Printf("开发模式：页面来自 %s", *vite)
	} else if _, err := os.Stat(filepath.Join(staticDirectory, "index.html")); err != nil {
		log.Printf("没有 web/dist，可先在 web 目录执行 npm run build，或用 -dev 连接 Vite")
	}

	router, err := newHTTPHandler(*dev, *vite, staticDirectory)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("ADM 预览监听 %s", *addr)
	if err := http.ListenAndServe(*addr, router); err != nil {
		log.Fatal(err)
	}
}

func newHTTPHandler(dev bool, viteAddress, staticDirectory string) (http.Handler, error) {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	router.HandleMethodNotAllowed = true
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}
	handle.RegisterRoutes(router, &service.PreviewService{})
	handle.RegisterReviewRoutes(router, &service.ReviewService{Client: jev.NewFromEnv(), Fetcher: &asset.Fetcher{}})

	var pages http.Handler = http.FileServer(http.Dir(staticDirectory))
	if dev {
		target, err := url.Parse(viteAddress)
		if err != nil {
			return nil, err
		}
		pages = httputil.NewSingleHostReverseProxy(target)
	}
	router.NoRoute(func(c *gin.Context) {
		// Vite proxies these prefixes back here; unmatched paths must not loop.
		if strings.HasPrefix(c.Request.URL.Path, "/api") || strings.HasPrefix(c.Request.URL.Path, "/mraid.js") {
			http.NotFound(c.Writer, c.Request)
			return
		}
		// Gin preselects 404; the page handler must choose the actual status.
		c.Status(http.StatusOK)
		pages.ServeHTTP(c.Writer, c.Request)
	})
	return router, nil
}
