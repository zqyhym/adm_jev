package jev

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNewFromEnvDefaults 验证未设置环境变量时使用默认 Hub 地址和模型。
func TestNewFromEnvDefaults(t *testing.T) {
	t.Setenv("AIHUB_BASE_URL", "")
	t.Setenv("AIHUB_MODEL", "")
	t.Setenv("AIHUB_API_KEY", "")
	client := NewFromEnv()
	if client.BaseURL != defaultBaseURL || client.Model != defaultModel || client.APIKey != "" {
		t.Fatalf("base=%s model=%s key=%q", client.BaseURL, client.Model, client.APIKey)
	}
	if !strings.HasSuffix(client.endpoint(), decisionPath) || strings.Contains(client.endpoint(), "/v1/v1/") {
		t.Fatal(client.endpoint())
	}
}

// TestDecideRequestShape 验证决策请求的路径、模型、鉴权头和图片内容符合约定。
func TestDecideRequestShape(t *testing.T) {
	const key = "phase0-secret-key"
	const ref = "a3f1c2e8-sample"
	photo := mustJPEG(t, 320, 320)
	got := make(chan *http.Request, 1)
	bodies := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		got <- r
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"jev-1.13.0","answers":{"camera_ad":{"type":"noul","noul":0.91}}}`))
	}))
	defer server.Close()

	result, err := (&Client{BaseURL: server.URL, Model: defaultModel, APIKey: key}).Decide(context.Background(), Request{
		State:             map[string]any{"creative": ref},
		Questions:         map[string]Question{"camera_ad": {Type: "noul", Instructions: "这是不是相机广告？"}},
		Images:            []ImageInput{{Ref: ref, Data: photo}},
		EnableImageHeader: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := <-got
	body := <-bodies
	if r.URL.Path != decisionPath {
		t.Fatalf("path=%s", r.URL.Path)
	}
	if r.Header.Get("Authorization") != "Bearer "+key {
		t.Fatalf("auth=%s", r.Header.Get("Authorization"))
	}
	if r.Header.Get("Content-Type") != "application/json" || r.Header.Get(imageHeader) != "true" {
		t.Fatalf("content=%s image=%s", r.Header.Get("Content-Type"), r.Header.Get(imageHeader))
	}
	if strings.Contains(string(body), key) {
		t.Fatal("请求体包含密钥")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["model"] != defaultModel {
		t.Fatalf("model=%v", payload["model"])
	}
	state := payload["state"].(map[string]any)
	if state["creative"] != ref {
		t.Fatalf("state=%v", state)
	}
	asset := payload["assets"].(map[string]any)[ref].(map[string]any)
	decoded, err := base64.StdEncoding.DecodeString(asset["data_base64"].(string))
	if err != nil || asset["format"] != "jpg" || !bytes.Equal(decoded, photo) {
		t.Fatalf("format=%v err=%v", asset["format"], err)
	}
	if result.Model != "jev-1.13.0" || result.Answers["camera_ad"].Status != StatusAnswered || *result.Answers["camera_ad"].Noul != 0.91 {
		t.Fatalf("%+v", result)
	}
}

// TestValidateImage 验证合格 JPEG 可通过，过小、超限、截断和 GIF 会被拒绝。
func TestValidateImage(t *testing.T) {
	if err := ValidateImage(mustJPEG(t, 320, 320)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImage(mustRead(t, "05_kodak_print_ad_1920.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImage(mustJPEG(t, 100, 100)); err == nil {
		t.Fatal("过小图片应被拒绝")
	}
	if err := ValidateImage(mustRead(t, "01_antique_shop_banner.jpg")); err == nil {
		t.Fatal("超限样本应被拒绝")
	}
	if err := ValidateImage(mustJPEG(t, 320, 320)[:20]); err == nil || !strings.Contains(err.Error(), "损坏") {
		t.Fatalf("截断图 err=%v", err)
	}
	var gifBuf bytes.Buffer
	if err := gif.Encode(&gifBuf, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImage(gifBuf.Bytes()); err == nil || !strings.Contains(err.Error(), "不是 JPEG 或 PNG") {
		t.Fatalf("gif err=%v", err)
	}
}

// TestDecideRejectsBadImageLocally 验证超限图片在本地被拒绝，不会发起 HTTP 请求。
func TestDecideRejectsBadImageLocally(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	_, err := (&Client{BaseURL: server.URL, Model: defaultModel, APIKey: "phase0-secret-key"}).Decide(context.Background(), Request{
		State:     map[string]any{"creative": "a3f1c2e8-bad"},
		Questions: map[string]Question{"camera_ad": {Type: "noul", Instructions: "这是不是相机广告？"}},
		Images:    []ImageInput{{Ref: "a3f1c2e8-bad", Data: mustRead(t, "01_antique_shop_banner.jpg")}},
	})
	if err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
	if strings.Contains(err.Error(), "NOT_HIT") {
		t.Fatal(err)
	}
}

// TestMissingAnswerStaysUnknown 验证响应缺少某个问题时保持 UNKNOWN，不补成未命中。
func TestMissingAnswerStaysUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"jev-1.13","answers":{"is_bug":{"type":"noul","noul":0.8}}}`))
	}))
	defer server.Close()
	result, err := (&Client{BaseURL: server.URL, Model: defaultModel, APIKey: "phase0-secret-key"}).Decide(context.Background(), Request{
		State: map[string]any{"ticket": "blank screen"},
		Questions: map[string]Question{
			"is_bug": {Type: "noul", Instructions: "Is this a bug?"},
			"team":   {Type: "choice", Instructions: "Which team?", Criteria: map[string]string{"frontend": "layout"}},
		},
	})
	var missing *MissingAnswersError
	if result == nil || !errors.As(err, &missing) || len(missing.Names) != 1 || missing.Names[0] != "team" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Answers["is_bug"].Status != StatusAnswered || result.Answers["team"].Status != StatusUnknown {
		t.Fatalf("%+v", result.Answers)
	}
	if strings.Contains(err.Error(), "NOT_HIT") {
		t.Fatal(err)
	}
}

// TestErrorRedactsAPIKey 验证错误响应里的密钥会被脱敏。
func TestErrorRedactsAPIKey(t *testing.T) {
	const key = "phase0-secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"` + key + `"}`))
	}))
	defer server.Close()
	_, err := (&Client{BaseURL: server.URL, Model: defaultModel, APIKey: key}).Decide(context.Background(), Request{
		State:     map[string]string{"ticket": "blank"},
		Questions: map[string]Question{"is_bug": {Type: "noul", Instructions: "Is this a bug?"}},
	})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusForbidden {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "[REDACTED]") || strings.Contains(err.Error(), "NOT_HIT") {
		t.Fatal(err)
	}
}

func mustJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(samplePath(name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func samplePath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return name
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "ad-creative-samples", name)
}
