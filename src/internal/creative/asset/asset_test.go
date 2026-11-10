package asset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"adm_jev/internal/creative/model"
	"adm_jev/internal/creative/preview"
	"adm_jev/internal/jev"
)

// TestFetchCreativeJPEG 验证创意图片下载成功后写入内容 hash，并按 sourceId 组装送审请求。
func TestFetchCreativeJPEG(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	var seen int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if r.URL.Path != "/creative.jpg" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Write(photo)
	}))
	defer server.Close()
	bids := []model.BidSource{{
		BidPath: "seatbid[0].bid[0]",
		Assets: []model.Asset{
			{SourceID: "seatbid[0].bid[0]#0", Kind: "image", Role: "creative", Value: server.URL + "/creative.jpg"},
			{SourceID: "seatbid[0].bid[0]#1", Kind: "text", Role: "creative", Value: "秋季促销"},
			{SourceID: "seatbid[0].bid[0]#2", Kind: "image", Role: "tracker", Value: server.URL + "/impression.php"},
		},
	}}
	files := Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
	if seen != 1 || !bids[0].Assets[0].Obtained || bids[0].Assets[2].Obtained {
		t.Fatalf("seen=%d assets=%+v", seen, bids[0].Assets)
	}
	sum := sha256.Sum256(photo)
	if bids[0].Assets[0].ContentHash != hex.EncodeToString(sum[:]) || !bytes.Equal(files["seatbid[0].bid[0]#0"], photo) {
		t.Fatalf("%+v", bids[0].Assets[0])
	}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "seatbid[0].bid[0]#0") || !strings.Contains(string(body), "秋季促销") || !strings.Contains(string(body), "Dating") {
			t.Errorf("body=%s", body)
		}
		answers := map[string]any{}
		for _, label := range visualLabels {
			answers[label.Name] = map[string]any{"type": "noul", "noul": 0.5}
		}
		raw, _ := json.Marshal(map[string]any{"model": "jev-1.13", "answers": answers})
		w.Write(raw)
	}))
	defer hub.Close()
	got, err := Submit(context.Background(), &jev.Client{BaseURL: hub.URL, Model: "jev-1.13", APIKey: "phase2-test"}, bids, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(visualLabels) {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.SourceID != "seatbid[0].bid[0]#0" || item.Status != StatusScored || item.Noul == nil || *item.Noul != 0.5 || item.Probability != nil {
			t.Fatalf("%+v", item)
		}
	}
}

// TestFetchRejectsBadBody 验证 404、截断图片和 HTML 都保持未取得并记录原因。
func TestFetchRejectsBadBody(t *testing.T) {
	cases := []struct {
		name string
		code int
		body []byte
	}{
		{"not found", http.StatusNotFound, []byte("missing")},
		{"truncated", http.StatusOK, []byte{0xFF, 0xD8, 0xFF}},
		{"html", http.StatusOK, []byte("<html>not image</html>")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write(tc.body)
			}))
			defer server.Close()
			bids := []model.BidSource{{Assets: []model.Asset{{
				SourceID: "img", Kind: "image", Role: "creative", Value: server.URL + "/a.jpg",
			}}}}
			Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
			if bids[0].Assets[0].Obtained || bids[0].Assets[0].FetchError == "" || strings.Contains(bids[0].Assets[0].FetchError, "NOT_HIT") {
				t.Fatalf("%+v", bids[0].Assets[0])
			}
		})
	}
}

// TestFetchRejectsRestrictedHost 验证本机、私网和云元数据地址不会被请求。
func TestFetchRejectsRestrictedHost(t *testing.T) {
	var called bool
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		t.Errorf("requested %s", r.URL)
		return nil, nil
	})}
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "a", Kind: "image", Role: "creative", Value: "http://127.0.0.1/a.jpg"},
		{SourceID: "b", Kind: "image", Role: "creative", Value: "http://10.0.0.1/a.jpg"},
		{SourceID: "c", Kind: "image", Role: "creative", Value: "http://169.254.169.254/latest/meta-data/"},
	}}}
	Fetch(context.Background(), bids, &Fetcher{HTTP: client})
	if called {
		t.Fatal("restricted host was requested")
	}
	for _, asset := range bids[0].Assets {
		if asset.Obtained || asset.FetchError == "" {
			t.Fatalf("%+v", asset)
		}
	}
}

// TestClassifySeparatesNotice 验证创意图的 HTTP 才进入未加密候选，通知地址分开统计。
func TestClassifySeparatesNotice(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<img src=\"http://cdn.example/a.jpg\" width=\"320\" height=\"50\">","nurl":"https://notice.example/win"}]}]}`
	bids, err := preview.Extract(input, "")
	if err != nil {
		t.Fatal(err)
	}
	var creative, notice int
	for _, report := range Classify(bids) {
		if report.URL == "http://cdn.example/a.jpg" {
			creative++
			if !report.Unsecured || report.Group != "creative" {
				t.Fatalf("%+v", report)
			}
		}
		if report.URL == "https://notice.example/win" {
			notice++
			if report.Unsecured || report.Group != "notice" {
				t.Fatalf("%+v", report)
			}
		}
	}
	if creative != 1 || notice != 1 {
		t.Fatalf("creative=%d notice=%d", creative, notice)
	}
}

// TestClassifyIgnoresHiddenHTTP 验证没有进入来源清单的 http 地址不会成为未加密候选。
func TestClassifyIgnoresHiddenHTTP(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<!-- http://hidden.example/a.jpg --><img src=\"https://cdn.example/a.jpg\" width=\"320\" height=\"50\">"}]}]}`
	bids, err := preview.Extract(input, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, report := range Classify(bids) {
		if strings.Contains(report.URL, "hidden.example") {
			t.Fatalf("%+v", report)
		}
	}
}

// TestSubmitSkipsWithoutImage 验证没有已取得的主体图片时不调用 Hub。
func TestSubmitSkipsWithoutImage(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	bids, err := preview.Extract(`{"seatbid":[{"bid":[{"adm":"<script src=\"mraid.js\"></script>","iurl":"https://cdn.example/preview.png"}]}]}`, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase2-test"}, bids, nil)
	if err == nil || called || !strings.Contains(err.Error(), "没有可送审的主体图片") {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

// TestFetchSkipsTrackerOnLiftoffShape 验证曝光地址不会被下载，预览图才会请求。
func TestFetchSkipsTrackerOnLiftoffShape(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Write(photo)
	}))
	defer server.Close()
	input := `{"seatbid":[{"bid":[{"w":320,"h":480,"adm":"<img src='` + server.URL + `/impression.php?id=1' style='position:absolute;left:-9999px'/><script src='mraid.js'></script>","iurl":"` + server.URL + `/preview.png"}]}]}`
	bids, err := preview.Extract(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if bids[0].ParseStatus != model.ParsePartial {
		t.Fatalf("%+v", bids[0])
	}
	Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
	if len(paths) != 1 || paths[0] != "/preview.png" {
		t.Fatalf("paths=%v", paths)
	}
}

func allowAll(string) error { return nil }

func mustJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
