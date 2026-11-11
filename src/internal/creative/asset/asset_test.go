package asset

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"adm_jev/internal/jev"
	"adm_jev/internal/material/parser"
)

// TestClassifySeparatesNotice 验证创意图的 HTTP 才进入未加密候选，通知地址分开统计。
func TestClassifySeparatesNotice(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<img src=\"http://cdn.example/a.jpg\" width=\"320\" height=\"50\">","nurl":"https://notice.example/win"}]}]}`
	bids, err := parser.Parse(input)
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
	bids, err := parser.Parse(input)
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
	bids, err := parser.Parse(`{"seatbid":[{"bid":[{"adm":"<script src=\"mraid.js\"></script>","iurl":"https://cdn.example/preview.png"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Submit(context.Background(), &jev.Client{BaseURL: server.URL, Model: "jev-1.13", APIKey: "phase2-test"}, bids, nil)
	if err == nil || called || !strings.Contains(err.Error(), "没有可送审的主体图片") {
		t.Fatalf("err=%v called=%v", err, called)
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
