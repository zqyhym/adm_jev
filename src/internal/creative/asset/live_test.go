package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"adm_jev/internal/creative/model"
	"adm_jev/internal/creative/preview"
	"adm_jev/internal/jev"
)

// TestLivePhase2Close 验证静态图和 Liftoff 预览图经下载后送入 Hub，判断能回到 sourceId。
func TestLivePhase2Close(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过内网收口")
	}
	client := jev.NewFromEnv()

	t.Run("static", func(t *testing.T) {
		photo := mustRead(t, filepath.Join(repoRoot(), "ad-creative-samples", "05_kodak_print_ad_1920.jpg"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(photo)
		}))
		defer server.Close()
		bids, err := preview.Extract(`{"seatbid":[{"bid":[{"crid":"kodak-1920","adm":"<div>Kodak</div>","iurl":"`+server.URL+`/preview.jpg"}]}]}`, "")
		if err != nil {
			t.Fatal(err)
		}
		files := Fetch(context.Background(), bids, &Fetcher{AllowHost: allowAll})
		image := imageBySuffix(t, bids[0], ".iurl")
		sum := sha256.Sum256(photo)
		if !image.Obtained || image.ContentHash != hex.EncodeToString(sum[:]) || string(files[image.SourceID]) != string(photo) {
			t.Fatalf("%+v", image)
		}
		answers, err := Submit(context.Background(), client, bids, files)
		if err != nil {
			t.Fatal(err)
		}
		if len(answers) != 1 || answers[0].SourceID != image.SourceID || answers[0].Noul < 0 || answers[0].Noul > 1 {
			t.Fatalf("%+v", answers)
		}
		t.Logf("source=%s noul=%v model=%s", answers[0].SourceID, answers[0].Noul, answers[0].Model)
	})

	t.Run("liftoff", func(t *testing.T) {
		raw := mustRead(t, filepath.Join(filepath.Dir(currentFile(t)), "testdata", "liftoff_bid.json"))
		bids, err := preview.Extract(string(raw), "")
		if err != nil {
			t.Fatal(err)
		}
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("提取后状态=%s", bids[0].ParseStatus)
		}
		blocked := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if !strings.Contains(r.URL.Host, "cdn.liftoff-creatives.io") {
				return nil, fmt.Errorf("拒绝非预览图请求 %s", r.URL.Host)
			}
			return http.DefaultTransport.RoundTrip(r)
		})}
		files := Fetch(context.Background(), bids, &Fetcher{HTTP: blocked})
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("下载后状态=%s，不能因为预览图而变成完整", bids[0].ParseStatus)
		}
		image := imageBySuffix(t, bids[0], ".iurl")
		if !image.Obtained {
			answers, err := Submit(context.Background(), client, bids, files)
			if err == nil || len(answers) != 0 {
				t.Fatalf("未取得预览图却送审了: answers=%+v err=%v fetch=%s", answers, err, image.FetchError)
			}
			t.Logf("预览图未取得，保持不完整: %s", image.FetchError)
			return
		}
		answers, err := Submit(context.Background(), client, bids, files)
		if err != nil {
			t.Fatal(err)
		}
		if len(answers) != 1 || answers[0].SourceID != image.SourceID || answers[0].Noul < 0 || answers[0].Noul > 1 {
			t.Fatalf("%+v", answers)
		}
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("送审后状态=%s", bids[0].ParseStatus)
		}
		t.Logf("source=%s noul=%v model=%s", answers[0].SourceID, answers[0].Noul, answers[0].Model)
	})
}

func imageBySuffix(t *testing.T, bid model.BidSource, suffix string) model.Asset {
	t.Helper()
	for _, asset := range bid.Assets {
		if asset.Kind == "image" && strings.HasSuffix(asset.FieldPath, suffix) {
			return asset
		}
	}
	t.Fatalf("没有 %s 图片: %+v", suffix, bid.Assets)
	return model.Asset{}
}

func currentFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("无法定位测试文件")
	}
	return file
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
