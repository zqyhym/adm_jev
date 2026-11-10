package handle

import (
	"bytes"
	"encoding/json"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/service"
)

func testRouter() *gin.Engine {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	RegisterRoutes(router, &service.PreviewService{})
	return router
}

// TestPreviewResult 验证预览接口返回 Banner 尺寸、文案和预览 HTML。
func TestPreviewResult(t *testing.T) {
	body := `{"input":"<img src='/api/empty' width='320' height='50'><div>秋季促销</div>"}`
	recorder := httptest.NewRecorder()
	testRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("content type=%s", recorder.Header().Get("Content-Type"))
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 9 || string(result["format"]) != `"banner"` || string(result["width"]) != "320" || string(result["height"]) != "50" {
		t.Fatalf("unexpected result: %s", recorder.Body)
	}
	if string(result["texts"]) != `["秋季促销"]` || string(result["blocked"]) != "null" {
		t.Fatalf("unexpected lists: %s", recorder.Body)
	}
	var previewHTML string
	if err := json.Unmarshal(result["previewHtml"], &previewHTML); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result["images"]), `"source":"tag"`) || !strings.Contains(previewHTML, "<!DOCTYPE html>") {
		t.Fatalf("missing preview content: %s", recorder.Body)
	}
}

// TestPreviewEmptyCollections 验证没有图片和文案时，列表字段返回 null。
func TestPreviewEmptyCollections(t *testing.T) {
	recorder := httptest.NewRecorder()
	testRouter().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(`{"input":"<div></div>"}`)))
	var result map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"images", "texts", "blocked"} {
		if string(result[name]) != "null" {
			t.Errorf("%s=%s; want null", name, result[name])
		}
	}
}

// TestPreviewErrors 验证非法请求体和空内容返回 400 及对应错误。
func TestPreviewErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
	}{
		{"invalid JSON", `{`, "请求体不是 JSON"},
		{"wrong input type", `{"input":1}`, "请求体不是 JSON"},
		{"empty input", `{"input":"  "}`, "内容为空"},
		{"missing input", `{}`, "内容为空"},
		{"missing adm", `{"input":"{\"seatbid\":[{\"bid\":[{}]}]}"}`, "没有 adm"},
		{"oversized body", `{"input":"` + strings.Repeat("x", 8<<20) + `"}`, "请求体不是 JSON"},
	}
	router := testRouter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(tc.body)))
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

// TestPreviewMethods 验证预览接口只接受 POST。
func TestPreviewMethods(t *testing.T) {
	router := testRouter()
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(method, "/api/preview", nil))
			if recorder.Code != http.StatusMethodNotAllowed || recorder.Body.String() != "只接受 POST\n" {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
		})
	}
}

// TestAuxiliaryResources 验证空图片和 mraid.js 可按多种方法访问。
func TestAuxiliaryResources(t *testing.T) {
	router := testRouter()
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			image := httptest.NewRecorder()
			router.ServeHTTP(image, httptest.NewRequest(method, "/api/empty", nil))
			if image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/gif" || image.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("image status=%d headers=%v", image.Code, image.Header())
			}
			decoded, err := gif.Decode(bytes.NewReader(image.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds().Dx() != 1 || decoded.Bounds().Dy() != 1 {
				t.Fatalf("image bounds=%v", decoded.Bounds())
			}
			script := httptest.NewRecorder()
			router.ServeHTTP(script, httptest.NewRequest(method, "/mraid.js", nil))
			if script.Code != http.StatusOK || script.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || script.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("script status=%d headers=%v", script.Code, script.Header())
			}
			if !strings.Contains(script.Body.String(), "window.mraid") || !strings.Contains(script.Body.String(), "adm-blocked") {
				t.Fatal("missing MRAID and interception behavior")
			}
		})
	}
}
