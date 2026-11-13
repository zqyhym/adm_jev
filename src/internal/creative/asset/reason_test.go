package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"testing"

	"adm_jev/internal/jev"
	"adm_jev/internal/material/model"
)

// TestSubmitUnknownReasons 验证每种未送审或未取得分数的情况都带上可归因的 Reason。
func TestSubmitUnknownReasons(t *testing.T) {
	gifBuf := bytes.Buffer{}
	if err := gif.Encode(&gifBuf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		assets []model.Asset
		files  map[string][]byte
		reason string
	}{
		{
			name:   "下载失败",
			assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: false, FetchError: "下载失败: status=404"}},
			reason: ReasonNotObtained,
		},
		{
			name:   "标记已取得但没有字节",
			assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}},
			files:  map[string][]byte{},
			reason: ReasonNoData,
		},
		{
			name:   "尺寸低于下限",
			assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}},
			files:  map[string][]byte{"img-1": mustJPEG(t, 320, 50)},
			reason: ReasonImageSize,
		},
		{
			name:   "格式不支持",
			assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}},
			files:  map[string][]byte{"img-1": gifBuf.Bytes()},
			reason: ReasonInvalidImage,
		},
		{
			name:   "图片截断",
			assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}},
			files:  map[string][]byte{"img-1": mustJPEG(t, 320, 320)[:20]},
			reason: ReasonInvalidImage,
		},
		{
			name:   "没有主体图片",
			assets: []model.Asset{{SourceID: "script-1", Kind: "script", Role: "dynamic", Value: "https://cdn.example/a.js"}},
			reason: ReasonNoSubject,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			defer server.Close()
			client := &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "reason-test"}
			labels, err := Submit(context.Background(), client, []model.BidSource{{Assets: item.assets}}, item.files)
			if err == nil {
				t.Fatal("未送审却返回成功")
			}
			if called {
				t.Fatal("未送审却请求了 Hub")
			}
			if len(labels) != len(visualLabels) {
				t.Fatalf("标签行数=%d", len(labels))
			}
			for _, label := range labels {
				if label.Status != StatusUnknown || label.Reason != item.reason || label.Noul != nil {
					t.Fatalf("%+v，期望 UNKNOWN/%s", label, item.reason)
				}
			}
		})
	}
}

// TestSubmitModelReasons 验证送审后 Hub 失败和标签缺项分别归因，且已评分行不带 Reason。
func TestSubmitModelReasons(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	assets := []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}}
	files := map[string][]byte{"img-1": photo}

	t.Run("Hub失败", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		client := &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "reason-test"}
		labels, err := Submit(context.Background(), client, []model.BidSource{{Assets: assets}}, files)
		if err == nil || len(labels) != len(visualLabels) {
			t.Fatalf("err=%v labels=%d", err, len(labels))
		}
		for _, label := range labels {
			if label.Status != StatusUnknown || label.Reason != ReasonHubFailed {
				t.Fatalf("%+v", label)
			}
		}
	})

	t.Run("标签缺项", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			answers := map[string]any{}
			for _, label := range visualLabels {
				if label.Name == "Violence" {
					continue
				}
				answers[label.Name] = map[string]any{"type": "noul", "noul": 0.2}
			}
			raw, _ := json.Marshal(map[string]any{"answers": answers})
			w.Write(raw)
		}))
		defer server.Close()
		client := &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "reason-test"}
		labels, err := Submit(context.Background(), client, []model.BidSource{{Assets: assets}}, files)
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range labels {
			if label.Label == "Violence" {
				if label.Status != StatusUnknown || label.Reason != ReasonMissingAnswer {
					t.Fatalf("%+v", label)
				}
				continue
			}
			if label.Status != StatusScored || label.Reason != "" {
				t.Fatalf("已评分行不应带 Reason: %+v", label)
			}
		}
	})
}

// TestSubmitKeepsUnsentImageInMultiImageBid 回归：同一 bid 里下载失败的图片也必须留下 10 行 UNKNOWN，
// 不能因为另一张图片送审成功就整张消失。
func TestSubmitKeepsUnsentImageInMultiImageBid(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answers := map[string]any{}
		for _, label := range visualLabels {
			answers[label.Name] = map[string]any{"type": "noul", "noul": 0.1}
		}
		raw, _ := json.Marshal(map[string]any{"answers": answers})
		w.Write(raw)
	}))
	defer server.Close()
	client := &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "reason-test"}
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "img-ok", Kind: "image", Role: "creative", Obtained: true},
		{SourceID: "img-failed", Kind: "image", Role: "creative", Obtained: false, FetchError: "下载失败: status=404"},
	}}}
	labels, err := Submit(context.Background(), client, bids, map[string][]byte{"img-ok": photo})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != len(visualLabels)*2 {
		t.Fatalf("标签行数=%d，期望 %d", len(labels), len(visualLabels)*2)
	}
	counts := map[string]int{}
	for _, label := range labels {
		counts[label.SourceID+"/"+label.Status+"/"+label.Reason]++
	}
	if counts["img-ok/SCORED/"] != len(visualLabels) {
		t.Fatalf("送审图片结果错误: %v", counts)
	}
	if counts["img-failed/UNKNOWN/"+ReasonNotObtained] != len(visualLabels) {
		t.Fatalf("下载失败图片结果错误: %v", counts)
	}
}
