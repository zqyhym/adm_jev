package handle

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/service"
	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
)

func reviewRouter(t *testing.T, client *jev.Client, fetcher *fetch.Fetcher) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	RegisterReviewRoutes(router, &service.ReviewService{Client: client, Fetcher: fetcher})
	return router
}

func reviewImage(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 320, 320))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func fakeReviewHub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Questions map[string]json.RawMessage `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		answers := make(map[string]any, len(payload.Questions))
		for name := range payload.Questions {
			answers[name] = map[string]any{"type": "noul", "noul": 0.8}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "fake-jev", "answers": answers})
	}))
}

func reviewRequestBody(t *testing.T, imageURL string) string {
	t.Helper()
	response := map[string]any{
		"seatbid": []any{map[string]any{
			"bid": []any{map[string]any{
				"crid":    "review-1",
				"adomain": []string{"example.com"},
				"w":       320,
				"h":       250,
				"impid":   "1",
				"adm":     "<img src='" + imageURL + "/a.png' width='320' height='250'><div>hello</div>",
			}},
		}},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"input": string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestReviewResult 验证审核接口返回素材状态、协议结论和十类标签分数。
func TestReviewResult(t *testing.T) {
	photo := reviewImage(t)
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(photo)
	}))
	defer imageServer.Close()
	hub := fakeReviewHub(t)
	defer hub.Close()
	client := &jev.Client{BaseURL: hub.URL, Model: "fake-jev", APIKey: "test", HTTP: hub.Client()}
	router := reviewRouter(t, client, &fetch.Fetcher{AllowHost: func(string) error { return nil }})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(reviewRequestBody(t, imageServer.URL))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	var result struct {
		Bids []struct {
			ParseStatus string `json:"parseStatus"`
			Assets      []struct {
				Kind        string `json:"kind"`
				Role        string `json:"role"`
				Obtained    bool   `json:"obtained"`
				ContentHash string `json:"contentHash"`
				FetchError  string `json:"fetchError"`
			} `json:"assets"`
		} `json:"bids"`
		URLs []struct {
			Group     string `json:"group"`
			Unsecured bool   `json:"unsecured"`
		} `json:"urls"`
		Unsecured struct {
			Status string `json:"status"`
		} `json:"unsecured"`
		Labels []struct {
			Status   string   `json:"status"`
			SourceID string   `json:"sourceId"`
			Noul     *float64 `json:"noul"`
		} `json:"labels"`
		Model       string `json:"model"`
		ReviewError string `json:"reviewError"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Bids) != 1 || len(result.URLs) != 1 || len(result.Labels) != 10 {
		t.Fatalf("unexpected counts: %s", recorder.Body)
	}
	if result.Bids[0].ParseStatus != "OK" {
		t.Fatalf("parse status=%s", result.Bids[0].ParseStatus)
	}
	var imageFound bool
	for _, item := range result.Bids[0].Assets {
		if item.Kind == "image" && item.Role == "creative" {
			imageFound = true
			if !item.Obtained || item.ContentHash == "" || item.FetchError != "" {
				t.Fatalf("image=%+v", item)
			}
		}
	}
	if !imageFound {
		t.Fatal("missing creative image")
	}
	if !result.URLs[0].Unsecured || result.URLs[0].Group != "creative" || result.Unsecured.Status != "HIT" {
		t.Fatalf("urls=%+v unsecured=%+v", result.URLs, result.Unsecured)
	}
	for _, label := range result.Labels {
		if label.Status != "SCORED" || label.Noul == nil || *label.Noul != 0.8 || label.SourceID == "" {
			t.Fatalf("label=%+v", label)
		}
	}
	if result.Model != "fake-jev" || result.ReviewError != "" {
		t.Fatalf("model=%s reviewError=%s", result.Model, result.ReviewError)
	}
}

// TestReviewWithoutHubKey 验证缺少模型密钥时保留素材结果并把标签标为 UNKNOWN。
func TestReviewWithoutHubKey(t *testing.T) {
	photo := reviewImage(t)
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(photo)
	}))
	defer imageServer.Close()
	client := &jev.Client{BaseURL: "http://127.0.0.1:1", Model: "fake-jev"}
	router := reviewRouter(t, client, &fetch.Fetcher{AllowHost: func(string) error { return nil }})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(reviewRequestBody(t, imageServer.URL))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	var result struct {
		Labels []struct {
			Status string   `json:"status"`
			Noul   *float64 `json:"noul"`
		} `json:"labels"`
		ReviewError string `json:"reviewError"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Labels) != 10 || !strings.Contains(result.ReviewError, "AIHUB_API_KEY") {
		t.Fatalf("labels=%+v reviewError=%s", result.Labels, result.ReviewError)
	}
	for _, label := range result.Labels {
		if label.Status != "UNKNOWN" || label.Noul != nil {
			t.Fatalf("label=%+v", label)
		}
	}
}

// TestReviewWithoutImage 验证没有创意图时不调用模型并返回可读告警。
func TestReviewWithoutImage(t *testing.T) {
	router := reviewRouter(t, &jev.Client{}, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(`{"input":"<div>hello</div>"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	var result struct {
		Bids []struct {
			Assets []struct {
				Kind string `json:"kind"`
			} `json:"assets"`
		} `json:"bids"`
		Labels      []json.RawMessage `json:"labels"`
		ReviewError string            `json:"reviewError"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Bids) != 1 || len(result.Labels) != 10 || !strings.Contains(result.ReviewError, "没有可送审的主体图片") {
		t.Fatalf("result=%s", recorder.Body)
	}
}

// TestReviewErrors 验证非法请求体和空内容返回 400。
func TestReviewErrors(t *testing.T) {
	router := reviewRouter(t, &jev.Client{}, nil)
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"invalid JSON", `{`, "请求体不是 JSON"},
		{"wrong input type", `{"input":1}`, "请求体不是 JSON"},
		{"empty input", `{"input":"  "}`, "内容为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(tc.body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			var result map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result) != 1 || result["error"] != tc.message {
				t.Fatalf("result=%v", result)
			}
		})
	}
}

// TestReviewMethods 验证审核接口只接受 POST。
func TestReviewMethods(t *testing.T) {
	router := reviewRouter(t, &jev.Client{}, nil)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(method, "/api/review", nil))
			if recorder.Code != http.StatusMethodNotAllowed || recorder.Body.String() != "只接受 POST\n" {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
		})
	}
}
