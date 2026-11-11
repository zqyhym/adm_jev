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

	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
	"adm_jev/internal/material/model"
	"adm_jev/internal/material/parser"
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
		bids, err := parser.Parse(`{"seatbid":[{"bid":[{"crid":"kodak-1920","adm":"<div>Kodak</div>","iurl":"` + server.URL + `/preview.jpg"}]}]}`)
		if err != nil {
			t.Fatal(err)
		}
		files := fetch.Fetch(context.Background(), bids, &fetch.Fetcher{AllowHost: allowAll})
		image := imageBySuffix(t, bids[0], ".iurl")
		sum := sha256.Sum256(photo)
		if !image.Obtained || image.ContentHash != hex.EncodeToString(sum[:]) || string(files[image.SourceID]) != string(photo) {
			t.Fatalf("%+v", image)
		}
		answers, err := Submit(context.Background(), client, bids, files)
		if err != nil {
			t.Fatal(err)
		}
		assertSourceLabels(t, answers, image.SourceID)
		t.Logf("source=%s labels=%d", image.SourceID, len(answers))
	})

	t.Run("liftoff", func(t *testing.T) {
		raw := mustRead(t, filepath.Join(filepath.Dir(currentFile(t)), "testdata", "liftoff_bid.json"))
		bids, err := parser.Parse(string(raw))
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
		files := fetch.Fetch(context.Background(), bids, &fetch.Fetcher{HTTP: blocked})
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("下载后状态=%s，不能因为预览图而变成完整", bids[0].ParseStatus)
		}
		image := imageBySuffix(t, bids[0], ".iurl")
		if !image.Obtained {
			answers, err := Submit(context.Background(), client, bids, files)
			if err == nil {
				t.Fatalf("未取得预览图却送审成功: %+v", answers)
			}
			for _, answer := range answers {
				if answer.Status != StatusUnknown {
					t.Fatalf("未取得图片却有判断: %+v", answer)
				}
			}
			t.Logf("预览图未取得，保持不完整: %s", image.FetchError)
			return
		}
		answers, err := Submit(context.Background(), client, bids, files)
		if err != nil {
			t.Fatal(err)
		}
		assertSourceLabels(t, answers, image.SourceID)
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("送审后状态=%s", bids[0].ParseStatus)
		}
		t.Logf("source=%s labels=%d", image.SourceID, len(answers))
	})
}

// TestLivePhase3Labels 用开发样本、柯达样图和 Liftoff 预览图做 11 类内网联调，并记录分数与未知状态。
func TestLivePhase3Labels(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过 11 类联调")
	}
	client := jev.NewFromEnv()
	root := repoRoot()
	images := []string{
		"gambling_lottery_ad_1902.jpg",
		"dating_lets_get_married_1926.jpg",
		"religion_billy_sunday_preaching.jpg",
		"violence_harvest_of_death_1863.jpg",
		"marijuana_hasheesh_ad_1880.png",
		"marijuana_cannabis_cigarette_ad_1881.jpg",
		"marijuana_medicine_bottle.jpg",
		"suggestive_moulin_rouge_poster_1891.jpg",
	}
	for _, name := range images {
		t.Run(name, func(t *testing.T) {
			logLiveLabels(t, client, name, mustRead(t, filepath.Join(root, "label-samples", name)), "")
		})
	}
	t.Run("kodak_1920", func(t *testing.T) {
		logLiveLabels(t, client, "kodak_1920", mustRead(t, filepath.Join(root, "ad-creative-samples", "05_kodak_print_ad_1920.jpg")), "")
	})
	t.Run("kodak_with_profane_text", func(t *testing.T) {
		copyText := string(mustRead(t, filepath.Join(root, "label-samples", "profane_language_1785.txt")))
		logLiveLabels(t, client, "kodak_with_profane_text", mustRead(t, filepath.Join(root, "ad-creative-samples", "05_kodak_print_ad_1920.jpg")), copyText)
	})
	t.Run("liftoff_iurl", func(t *testing.T) {
		raw := mustRead(t, filepath.Join(filepath.Dir(currentFile(t)), "testdata", "liftoff_bid.json"))
		bids, err := parser.Parse(string(raw))
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
		files := fetch.Fetch(context.Background(), bids, &fetch.Fetcher{HTTP: blocked})
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("下载后状态=%s", bids[0].ParseStatus)
		}
		image := imageBySuffix(t, bids[0], ".iurl")
		if !image.Obtained {
			t.Fatalf("预览图未取得: %s", image.FetchError)
		}
		labels, err := Submit(context.Background(), client, bids, files)
		if err != nil {
			t.Fatal(err)
		}
		logLabelRows(t, "liftoff_iurl", labels, image.SourceID)
		logUnsecured(t, bids)
		if bids[0].ParseStatus != model.ParsePartial {
			t.Fatalf("送审后状态=%s", bids[0].ParseStatus)
		}
	})
}

func logLiveLabels(t *testing.T, client *jev.Client, name string, photo []byte, copyText string) {
	t.Helper()
	sourceID := "sample-" + name
	assets := []model.Asset{{
		SourceID:  sourceID,
		Kind:      "image",
		Role:      "creative",
		Value:     "https://label-samples.local/" + name,
		FieldPath: "seatbid[0].bid[0].iurl",
		Obtained:  true,
	}}
	if copyText != "" {
		assets = append(assets, model.Asset{SourceID: sourceID + "-copy", Kind: "text", Role: "creative", Value: copyText})
	}
	bids := []model.BidSource{{BidPath: "seatbid[0].bid[0]", ParseStatus: model.ParseOK, Assets: assets}}
	labels, err := Submit(context.Background(), client, bids, map[string][]byte{sourceID: photo})
	if err != nil {
		t.Fatal(err)
	}
	logLabelRows(t, name, labels, sourceID)
	logUnsecured(t, bids)
}

func logLabelRows(t *testing.T, name string, labels []LabelResult, sourceID string) {
	t.Helper()
	matched := 0
	for _, label := range labels {
		if label.SourceID != sourceID {
			continue
		}
		matched++
		if label.Status == StatusHit || label.Status == StatusNotHit || label.Probability != nil {
			t.Fatalf("%s %+v", name, label)
		}
		if label.Status == StatusScored {
			if label.Noul == nil || *label.Noul < 0 || *label.Noul > 1 {
				t.Fatalf("%s %+v", name, label)
			}
			t.Logf("%s\t%s\tSCORED\t%.2f", name, label.Label, *label.Noul)
			continue
		}
		t.Logf("%s\t%s\tUNKNOWN", name, label.Label)
	}
	if matched != len(visualLabels) {
		t.Fatalf("%s labels=%d", name, matched)
	}
}

func logUnsecured(t *testing.T, bids []model.BidSource) {
	t.Helper()
	got := UnsecuredRequest(bids)
	t.Logf("unsecured=%s evidence=%d notices=%d", got.Status, len(got.Evidence), len(got.Notices))
}

func assertSourceLabels(t *testing.T, answers []LabelResult, sourceID string) {
	t.Helper()
	matched := 0
	for _, answer := range answers {
		if answer.SourceID != sourceID {
			continue
		}
		matched++
		if answer.Status == StatusHit || answer.Status == StatusNotHit || answer.Probability != nil {
			t.Fatalf("%+v", answer)
		}
		if answer.Status == StatusScored && (answer.Noul == nil || *answer.Noul < 0 || *answer.Noul > 1) {
			t.Fatalf("%+v", answer)
		}
	}
	if matched != len(visualLabels) {
		t.Fatalf("labels=%d %+v", matched, answers)
	}
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
