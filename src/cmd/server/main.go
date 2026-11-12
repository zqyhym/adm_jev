package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/handle"
	"adm_jev/internal/creative/service"
	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
	"adm_jev/internal/platform/config"
	platformmongo "adm_jev/internal/platform/mongo"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
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

	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, database, err := platformmongo.Connect(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := client.Disconnect(closeCtx); closeErr != nil {
			log.Printf("关闭 MongoDB 连接失败: %v", closeErr)
		}
	}()
	log.Printf("MongoDB 已连接: %s", database.Name())

	router, err := newHTTPHandler(*dev, *vite, staticDirectory)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverError := make(chan error, 1)
	go func() {
		log.Printf("ADM 预览监听 %s", *addr)
		serverError <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err = <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
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
	handle.RegisterReviewRoutes(router, &service.ReviewService{Client: jev.NewFromEnv(), Fetcher: &fetch.Fetcher{}})

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
