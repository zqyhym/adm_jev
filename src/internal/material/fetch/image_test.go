package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"adm_jev/internal/material/model"
	"adm_jev/internal/material/parser"
)

// TestFetchCreativeJPEG 验证创意图片下载成功后写入内容 hash，追踪像素不会发出请求。
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

// TestFetchKeepsSmallImage 验证小于送审尺寸的标准 Banner 仍然算已取得。
func TestFetchKeepsSmallImage(t *testing.T) {
	photo := mustJPEG(t, 320, 50)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(photo)
	}))
	defer server.Close()
	bids := []model.BidSource{{Assets: []model.Asset{{
		SourceID: "img", Kind: "image", Role: "creative", Value: server.URL + "/banner.jpg",
	}}}}
	files := Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
	sum := sha256.Sum256(photo)
	if !bids[0].Assets[0].Obtained || bids[0].Assets[0].ContentHash != hex.EncodeToString(sum[:]) || !bytes.Equal(files["img"], photo) {
		t.Fatalf("%+v", bids[0].Assets[0])
	}
}

// TestFetchSkipsStylesheetAndTracker 验证样式表和曝光地址不会被下载，只有创意图片会请求。
func TestFetchSkipsStylesheetAndTracker(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Write(photo)
	}))
	defer server.Close()
	input := `{"seatbid":[{"bid":[{"adm":"<link rel=\"stylesheet\" href=\"` + server.URL + `/a.css\"><img src='` + server.URL + `/impression.php?id=1' style='position:absolute;left:-9999px'/><img src=\"` + server.URL + `/banner.jpg\">"}]}]}`
	bids, err := parser.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
	if len(paths) != 1 || paths[0] != "/banner.jpg" {
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
