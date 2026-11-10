package jev

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestLivePhase0 验证内网 Hub 的文本、单图、换图、去图、图文、图片请求头、超限图和错误密钥。
func TestLivePhase0(t *testing.T) {
	key := os.Getenv("AIHUB_API_KEY")
	if key == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过内网联调")
	}
	client := NewFromEnv()
	ctx := context.Background()
	var cameraNoul float64

	t.Run("text", func(t *testing.T) {
		result, err := client.Decide(ctx, Request{
			State: map[string]any{
				"customer_tier": "enterprise",
				"ticket":        "My checkout page shows a blank screen after I click Pay.",
			},
			Questions: map[string]Question{
				"is_bug": {
					Type:         "noul",
					Instructions: "Is the customer reporting a software defect?",
					Criteria:     map[string]string{"true": "broken behavior", "false": "question or feature"},
				},
				"team": {
					Type:         "choice",
					Instructions: "Which team should own this ticket?",
					Criteria:     map[string]string{"account": "login", "frontend": "layout", "payments": "checkout"},
				},
				"urgency": {
					Type:         "score",
					Instructions: "How urgent is this ticket?",
					Criteria:     []string{"next release", "this week", "blocking revenue"},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertNoHitLabel(t, result)
		if result.Answers["is_bug"].Noul == nil || result.Answers["team"].Choice == "" || result.Answers["urgency"].Score == nil {
			t.Fatalf("%+v", result.Answers)
		}
		t.Logf("model=%q is_bug=%v team=%s urgency=%v", result.Model, *result.Answers["is_bug"].Noul, result.Answers["team"].Choice, *result.Answers["urgency"].Score)
	})

	t.Run("single_image", func(t *testing.T) {
		result, err := client.Decide(ctx, cameraRequest("a3f1c2e8-kodak-1920", mustRead(t, "05_kodak_print_ad_1920.jpg"), true))
		if err != nil {
			t.Fatal(err)
		}
		cameraNoul = noulOf(t, result, "camera_ad")
		if cameraNoul < 0.7 {
			t.Fatalf("相机广告 noul=%v，预期 >= 0.7", cameraNoul)
		}
		t.Logf("model=%q camera_noul=%v", result.Model, cameraNoul)
	})

	t.Run("swap_image", func(t *testing.T) {
		result, err := client.Decide(ctx, cameraRequest("a3f1c2e8-radio-1939", mustRead(t, "04_wjr_wgar_radio_ad_1939.jpg"), true))
		if err != nil {
			t.Fatal(err)
		}
		radioNoul := noulOf(t, result, "camera_ad")
		if radioNoul > 0.3 || radioNoul >= cameraNoul {
			t.Fatalf("电台广告 noul=%v，相机广告 noul=%v", radioNoul, cameraNoul)
		}
		t.Logf("model=%q radio_noul=%v", result.Model, radioNoul)
	})

	t.Run("remove_image", func(t *testing.T) {
		result, err := client.Decide(ctx, Request{
			State:             map[string]any{"creative": "未提供图片"},
			Questions:         cameraQuestions(),
			EnableImageHeader: true,
		})
		if err != nil {
			t.Logf("去图请求未给出高概率: %v", err)
			return
		}
		value := noulOf(t, result, "camera_ad")
		if value >= cameraNoul {
			t.Fatalf("去图 noul=%v 不低于有图 noul=%v", value, cameraNoul)
		}
		t.Logf("去图 noul=%v", value)
	})

	t.Run("image_and_text", func(t *testing.T) {
		const ref = "a3f1c2e8-kodak-1921"
		result, err := client.Decide(ctx, Request{
			State: map[string]any{"creative": ref, "copy": "Eastman Kodak"},
			Questions: map[string]Question{
				"camera_copy": {
					Type:         "noul",
					Instructions: "图片和文案是不是都在推广相机？请结合 `creative` 与 `copy` 判断。",
					Criteria: map[string]string{
						"true":  "图片和文案都在推广相机",
						"false": "图片或文案不是相机推广",
					},
				},
			},
			Images:            []ImageInput{{Ref: ref, Data: mustRead(t, "02_kodak_print_ad_1921.jpg")}},
			EnableImageHeader: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		value := noulOf(t, result, "camera_copy")
		t.Logf("model=%q image_text_noul=%v", result.Model, value)
	})

	t.Run("header_off", func(t *testing.T) {
		result, err := client.Decide(ctx, cameraRequest("a3f1c2e8-kodak-header", mustRead(t, "05_kodak_print_ad_1920.jpg"), false))
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			t.Logf("不带图片头被拒绝 status=%d", httpErr.Status)
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("不带图片头被接受 noul=%v model=%q", noulOf(t, result, "camera_ad"), result.Model)
	})

	t.Run("oversize_image", func(t *testing.T) {
		_, err := client.post(ctx, cameraRequest("a3f1c2e8-oversize", mustRead(t, "01_antique_shop_banner.jpg"), true))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status < 400 {
			t.Fatalf("超限图片应被 Hub 拒绝: %v", err)
		}
		if strings.Contains(err.Error(), "NOT_HIT") {
			t.Fatal(err)
		}
		t.Logf("超限图片 status=%d", httpErr.Status)
	})

	t.Run("bad_key", func(t *testing.T) {
		bad := NewFromEnv()
		bad.APIKey = "invalid-phase0-key"
		_, err := bad.Decide(ctx, Request{
			State:     map[string]string{"ticket": "blank screen"},
			Questions: map[string]Question{"is_bug": {Type: "noul", Instructions: "Is the customer reporting a software defect?"}},
		})
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status < http.StatusBadRequest {
			t.Fatal(err)
		}
		if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "NOT_HIT") {
			t.Fatal("错误信息包含密钥或未命中结论")
		}
		t.Logf("错误密钥 status=%d", httpErr.Status)
	})
}

func cameraQuestions() map[string]Question {
	return map[string]Question{
		"camera_ad": {
			Type:         "noul",
			Instructions: "这是不是相机广告？请只根据 `creative` 中的图片判断。",
			Criteria: map[string]string{
				"true":  "画面是相机或摄影产品的广告",
				"false": "画面不是相机广告",
			},
		},
	}
}

func cameraRequest(ref string, data []byte, header bool) Request {
	return Request{
		State:             map[string]any{"creative": ref},
		Questions:         cameraQuestions(),
		Images:            []ImageInput{{Ref: ref, Data: data}},
		EnableImageHeader: header,
	}
}

func noulOf(t *testing.T, result *Result, name string) float64 {
	t.Helper()
	answer := result.Answers[name]
	if answer.Status != StatusAnswered || answer.Noul == nil {
		t.Fatalf("%s 未解析: %+v", name, answer)
	}
	assertNoHitLabel(t, result)
	return *answer.Noul
}

func assertNoHitLabel(t *testing.T, result *Result) {
	t.Helper()
	for _, answer := range result.Answers {
		if answer.Status != StatusAnswered && answer.Status != StatusUnknown {
			t.Fatalf("意外状态 %+v", answer)
		}
	}
}
