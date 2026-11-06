package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

	"adm_jev/internal/adm"
)

func main() {
	addr := flag.String("addr", ":8080", "监听地址")
	dev := flag.Bool("dev", false, "把页面请求代理到 Vite")
	vite := flag.String("vite", "http://127.0.0.1:5173", "Vite 地址")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/preview", previewHandler)
	mux.HandleFunc("/api/empty", emptyHandler)
	mux.HandleFunc("/mraid.js", mraidHandler)
	if *dev {
		target, err := url.Parse(*vite)
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle("/", httputil.NewSingleHostReverseProxy(target))
		log.Printf("开发模式：页面来自 %s", target)
	} else {
		dist := filepath.Join("web", "dist")
		if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
			log.Printf("没有 web/dist，可先在 web 目录执行 npm run build，或用 -dev 连接 Vite")
		}
		mux.Handle("/", http.FileServer(http.Dir(dist)))
	}

	log.Printf("ADM 预览监听 %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只接受 POST", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Input string `json:"input"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体不是 JSON"})
		return
	}
	result, err := adm.Parse(req.Input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func emptyHandler(w http.ResponseWriter, r *http.Request) {
	// 1x1 透明图，避免被改写的监测像素显示成破图。
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	})
}

func mraidHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(adm.MraidJS()))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
