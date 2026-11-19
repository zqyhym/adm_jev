package asset

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"adm_jev/internal/jev"
	"adm_jev/internal/material/model"
)

// TestReviewsAsksAllLabels 验证已取得图片的请求包含全部标签、命中排除要点和 sourceId。
func TestReviewsAsksAllLabels(t *testing.T) {
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true},
		{SourceID: "copy-1", Kind: "text", Role: "creative", Value: "秋季促销"},
	}}}
	requests := Reviews(bids, map[string][]byte{"img-1": mustJPEG(t, 320, 320)})
	if len(requests) != 1 || len(requests[0].Questions) != len(visualLabels) {
		t.Fatalf("%d", len(requests))
	}
	state := requests[0].State.(map[string]any)
	if state["sourceId"] != "img-1" || state["copy"] != "秋季促销" {
		t.Fatalf("%+v", state)
	}
	for _, label := range visualLabels {
		question := requests[0].Questions[label.Name]
		if question.Type != "noul" || !strings.Contains(question.Instructions, label.Name) {
			t.Fatalf("%s %+v", label.Name, question)
		}
		criteria := question.Criteria.(map[string]string)
		if criteria["true"] != label.Hit || criteria["false"] != label.Exclude {
			t.Fatalf("%s %+v", label.Name, criteria)
		}
	}
}

// TestSubmitMissingLabelStaysUnknown 验证假 Hub 少返回一个标签时，只有该类为 UNKNOWN。
func TestSubmitMissingLabelStaysUnknown(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answers := map[string]any{}
		for _, label := range visualLabels {
			if label.Name == "Violence" {
				continue
			}
			answers[label.Name] = map[string]any{"type": "noul", "noul": 0.2}
		}
		raw, _ := json.Marshal(map[string]any{"model": "jev-1.13", "answers": answers})
		w.Write(raw)
	}))
	defer server.Close()
	bids := []model.BidSource{{Assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}}}}
	got, err := Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase3-test"}, bids, map[string][]byte{"img-1": photo})
	if err != nil {
		t.Fatal(err)
	}
	scored := 0
	for _, item := range got {
		if item.UpstreamModel != "jev-1.13" {
			t.Fatalf("回显模型未传播: %+v", item)
		}
		if item.Status == StatusNotHit || item.Status == StatusHit || item.Probability != nil {
			t.Fatalf("%+v", item)
		}
		if item.Label == "Violence" {
			if item.Status != StatusUnknown || item.Noul != nil {
				t.Fatalf("%+v", item)
			}
			continue
		}
		if item.Status != StatusScored || item.Noul == nil || *item.Noul != 0.2 {
			t.Fatalf("%+v", item)
		}
		scored++
	}
	if scored != len(visualLabels)-1 {
		t.Fatalf("scored=%d", scored)
	}
}

// TestSubmitWithoutImageDoesNotCallHub 验证没有已取得图片时不请求 Hub，视觉标签为 UNKNOWN。
func TestSubmitWithoutImageDoesNotCallHub(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	bids := []model.BidSource{{Assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: false}}}}
	got, err := Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase3-test"}, bids, nil)
	if err == nil || called || len(got) != len(visualLabels) {
		t.Fatalf("err=%v called=%v labels=%d", err, called, len(got))
	}
	for _, item := range got {
		if item.Status != StatusUnknown || item.Noul != nil || item.SourceID != "img-1" {
			t.Fatalf("%+v", item)
		}
	}
}

// TestUnsecuredHitsCreativeHTTP 验证只有创意资源的 http 地址使 Unsecured Request 命中。
func TestUnsecuredHitsCreativeHTTP(t *testing.T) {
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "img-1", Kind: "image", Role: "creative", Value: "http://cdn.example/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
		{SourceID: "notice-1", Kind: "url", Role: "creative", Value: "https://notice.example/win", FieldPath: "seatbid[0].bid[0].nurl"},
	}}}
	got := UnsecuredRequest(bids)
	if got.Status != StatusHit || len(got.Evidence) != 1 || got.Evidence[0].SourceID != "img-1" {
		t.Fatalf("%+v", got)
	}
	if len(got.Notices) != 1 || got.Notices[0].URL != "https://notice.example/win" {
		t.Fatalf("%+v", got.Notices)
	}
}

// TestUnsecuredIgnoresNoticeHTTP 验证创意地址都是 https 时，http 通知地址不并入未加密结论。
func TestUnsecuredIgnoresNoticeHTTP(t *testing.T) {
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "img-1", Kind: "image", Role: "creative", Value: "https://cdn.example/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
		{SourceID: "notice-1", Kind: "url", Role: "creative", Value: "http://notice.example/win", FieldPath: "seatbid[0].bid[0].nurl"},
	}}}
	got := UnsecuredRequest(bids)
	if got.Status != StatusNotHit || len(got.Evidence) != 0 || len(got.Notices) != 1 {
		t.Fatalf("%+v", got)
	}
}

// TestSubmitKeepsResultsBySource 验证同一 bid 的两张图片按各自 sourceId 保留标签。
func TestSubmitKeepsResultsBySource(t *testing.T) {
	photo := mustJPEG(t, 320, 320)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioRead(r)
		source := "img-a"
		if strings.Contains(body, "img-b") {
			source = "img-b"
		}
		answers := map[string]any{}
		score := 0.1
		if source == "img-b" {
			score = 0.2
		}
		for _, label := range visualLabels {
			answers[label.Name] = map[string]any{"type": "noul", "noul": score}
		}
		raw, _ := json.Marshal(map[string]any{"answers": answers})
		w.Write(raw)
	}))
	defer server.Close()
	bids := []model.BidSource{{Assets: []model.Asset{
		{SourceID: "img-a", Kind: "image", Role: "creative", Obtained: true},
		{SourceID: "img-b", Kind: "image", Role: "creative", Obtained: true},
	}}}
	got, err := Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase3-test"}, bids, map[string][]byte{
		"img-a": photo,
		"img-b": photo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(visualLabels)*2 {
		t.Fatalf("len=%d", len(got))
	}
	for _, item := range got {
		want := 0.1
		if item.SourceID == "img-b" {
			want = 0.2
		}
		if item.Status != StatusScored || item.Noul == nil || *item.Noul != want {
			t.Fatalf("%+v", item)
		}
	}
}

// TestSubmitSkipsUndersizedImage 验证已取得但低于送审尺寸的图片不调用 Hub，标签为 UNKNOWN。
func TestSubmitSkipsUndersizedImage(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	photo := mustJPEG(t, 320, 50)
	bids := []model.BidSource{{Assets: []model.Asset{{SourceID: "img-1", Kind: "image", Role: "creative", Obtained: true}}}}
	got, err := Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase3-test"}, bids, map[string][]byte{"img-1": photo})
	if err == nil || called || len(got) != len(visualLabels) {
		t.Fatalf("err=%v called=%v labels=%d", err, called, len(got))
	}
	for _, item := range got {
		if item.Status != StatusUnknown || item.Noul != nil || item.SourceID != "img-1" {
			t.Fatalf("%+v", item)
		}
	}
}

func ioRead(r *http.Request) (string, error) {
	body, err := io.ReadAll(r.Body)
	return string(body), err
}
