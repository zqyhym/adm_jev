package asset

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"adm_jev/internal/jev"
	"adm_jev/internal/material/model"
)

// 图像通路对照（相机/电台、choice、纯色）是回归门禁：Hub 读图能力退化时直接失败。
// 标签对照臂（TestLiveLabelArms）仍是诊断测量，只记录不失败。
// 所有对照臂必须使用中性图片引用。sourceId 和 ref 会进入模型可见文本，
// 带标签词的命名（如 probe-gambling_xxx）会把答案泄漏给模型，使分数虚高。

// TestLiveImageChannelControl 用相机/电台两张图对照，测量画面内容是否影响 noul。
func TestLiveImageChannelControl(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过图像通路对照")
	}
	client := jev.NewFromEnv()
	root := repoRoot()
	question := jev.Question{
		Type:         "noul",
		Instructions: "请只根据 `creative` 中的图片判断这是不是相机或摄影产品广告。",
		Criteria:     map[string]string{"true": "相机或摄影产品广告", "false": "不是相机广告"},
	}
	camera := probeScore(t, client, "camera_ad", question,
		filepath.Join(root, "ad-creative-samples", "05_kodak_print_ad_1920.jpg"), "")
	radio := probeScore(t, client, "camera_ad", question,
		filepath.Join(root, "ad-creative-samples", "04_wjr_wgar_radio_ad_1939.jpg"), "")
	t.Logf("图像通路对照(noul)\t相机广告=%.3f\t电台广告=%.3f\t差值=%.3f", camera, radio, camera-radio)
	if camera-radio < 0.3 {
		t.Errorf("图像通路门禁失败：换图后 noul 几乎不变（相机=%.3f 电台=%.3f），画面内容未参与判断", camera, radio)
	}
}

// TestLiveImageChoiceControl 用 choice 型问题判别画面内容，区分"图像未到达"和"noul 不敏感"。
func TestLiveImageChoiceControl(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过 choice 图像对照")
	}
	client := jev.NewFromEnv()
	root := repoRoot()
	question := jev.Question{
		Type:         "choice",
		Instructions: "请只根据 `creative` 中的图片判断这张广告宣传的是什么。",
		Criteria:     map[string]string{"camera": "相机或摄影产品", "radio": "广播电台", "other": "其他"},
	}
	camera := filepath.Join(root, "ad-creative-samples", "05_kodak_print_ad_1920.jpg")
	radio := filepath.Join(root, "ad-creative-samples", "04_wjr_wgar_radio_ad_1939.jpg")
	cameraChoice := probeChoice(t, client, question, camera, true)
	radioChoice := probeChoice(t, client, question, radio, true)
	t.Logf("choice对照\t相机图=%s\t电台图=%s", cameraChoice, radioChoice)
	if cameraChoice != "camera" || radioChoice != "radio" {
		t.Errorf("图像通路门禁失败：choice 未判别画面内容（相机图=%s 电台图=%s）", cameraChoice, radioChoice)
	}
	t.Logf("图片头对照\t相机图带头=%s\t相机图不带头=%s",
		cameraChoice, probeChoice(t, client, question, camera, false))
}

// TestLiveLabelArms 对指定正例做全量问/单问/去图/换图/强文案对照，定位标签低分的原因。
func TestLiveLabelArms(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过标签对照臂")
	}
	client := jev.NewFromEnv()
	root := repoRoot()
	cases := []struct {
		sample string
		image  string
		label  string
		copy   string
	}{
		{"gambling_lottery_1902", "label-samples/gambling_lottery_ad_1902.jpg", "Gambling",
			"CASINO 老虎机 注册送彩金 立即下注 赢钱提现"},
		{"marijuana_hasheesh_1880", "label-samples/marijuana_hasheesh_ad_1880.png", "Marijuana",
			"大麻 THC 药房 吸食制品 促销"},
		{"violence_harvest_of_death_1863", "label-samples/violence_harvest_of_death_1863.jpg", "Violence", ""},
	}
	swap := filepath.Join(root, "ad-creative-samples", "05_kodak_print_ad_1920.jpg")
	for _, item := range cases {
		target := labelByName(t, item.label)
		photo := filepath.Join(root, filepath.FromSlash(item.image))
		question := jev.Question{
			Type:         "noul",
			Instructions: "请只根据 `creative` 中的图片和同条请求中的文案判断是否属于 " + target.Name + "。",
			Criteria:     map[string]string{"true": target.Hit, "false": target.Exclude},
		}

		ten := productionScore(t, client, photo, item.label)
		single := probeScore(t, client, target.Name, question, photo, "")
		noImage := probeScoreNoImage(t, client, target.Name, question)
		swapped := probeScore(t, client, target.Name, question, swap, "")
		withCopy := "-"
		if item.copy != "" {
			withCopy = formatScore(probeScore(t, client, target.Name, question, photo, item.copy))
		}
		t.Logf("%s\t%s\t%d问=%s\t单问=%s\t去图=%s\t换图=%s\t单问加强文案=%s",
			item.sample, item.label, len(visualLabels), formatScore(ten), formatScore(single),
			formatScore(noImage), formatScore(swapped), withCopy)
	}
}

// productionScore 走生产路径（全量标签一次请求），用中性 sourceId 避免标签泄漏。
func productionScore(t *testing.T, client *jev.Client, photo, labelName string) float64 {
	t.Helper()
	data, err := os.ReadFile(photo)
	if err != nil {
		t.Fatal(err)
	}
	const sourceID = "probe-image"
	bids := []model.BidSource{{Assets: []model.Asset{{
		SourceID: sourceID, Kind: "image", Role: "creative", Obtained: true,
	}}}}
	labels, err := Submit(context.Background(), client, bids, map[string][]byte{sourceID: data})
	if err != nil {
		t.Logf("生产路径错误: %v", err)
	}
	for _, label := range labels {
		if label.Label != labelName {
			continue
		}
		if label.Noul == nil {
			t.Logf("标签 %s 状态 %s 无分数", labelName, label.Status)
			return -1
		}
		return *label.Noul
	}
	return -1
}

func labelByName(t *testing.T, name string) visualLabel {
	t.Helper()
	for _, label := range visualLabels {
		if label.Name == name {
			return label
		}
	}
	t.Fatalf("没有标签 %s", name)
	return visualLabel{}
}

func probeScore(t *testing.T, client *jev.Client, name string, question jev.Question, photo, copyText string) float64 {
	t.Helper()
	data, err := os.ReadFile(photo)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]any{"creative": "image-1", "sourceId": "probe"}
	if copyText != "" {
		state["copy"] = copyText
	}
	return decideProbe(t, client, name, question, state, []jev.ImageInput{{Ref: "image-1", Data: data}})
}

func probeScoreNoImage(t *testing.T, client *jev.Client, name string, question jev.Question) float64 {
	t.Helper()
	state := map[string]any{"creative": "未提供图片", "sourceId": "probe"}
	return decideProbe(t, client, name, question, state, nil)
}

func decideProbe(t *testing.T, client *jev.Client, name string, question jev.Question, state map[string]any, images []jev.ImageInput) float64 {
	t.Helper()
	result, err := client.Decide(context.Background(), jev.Request{
		State:             state,
		Questions:         map[string]jev.Question{name: question},
		Images:            images,
		EnableImageHeader: true,
	})
	if err != nil {
		t.Logf("对照臂 %s 错误: %v", name, err)
	}
	if result == nil {
		return -1
	}
	answer, ok := result.Answers[name]
	if !ok || answer.Noul == nil {
		return -1
	}
	return *answer.Noul
}

func probeChoice(t *testing.T, client *jev.Client, question jev.Question, photo string, header bool) string {
	t.Helper()
	data, err := os.ReadFile(photo)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Decide(context.Background(), jev.Request{
		State:             map[string]any{"creative": "image-1"},
		Questions:         map[string]jev.Question{"product": question},
		Images:            []jev.ImageInput{{Ref: "image-1", Data: data}},
		EnableImageHeader: header,
	})
	if err != nil {
		return "错误:" + err.Error()
	}
	if result == nil {
		return "无结果"
	}
	answer := result.Answers["product"]
	if answer.Status != jev.StatusAnswered || answer.Choice == "" {
		return "状态:" + answer.Status
	}
	return answer.Choice
}

// TestLiveSolidColorControl 用纯红/纯蓝两张合成图做零文本先验对照。
// 画面若真的参与判断，红色图的"主色是否红色"必须显著高于蓝色图。
func TestLiveSolidColorControl(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过纯色对照")
	}
	client := jev.NewFromEnv()
	question := jev.Question{
		Type:         "noul",
		Instructions: "请只根据 `creative` 中的图片判断画面主色是否为红色。",
		Criteria:     map[string]string{"true": "画面以红色为主", "false": "画面不以红色为主"},
	}
	red := probeScoreBytes(t, client, "red", question, solidPNG(t, color.RGBA{R: 220, G: 20, B: 20, A: 255}))
	blue := probeScoreBytes(t, client, "red", question, solidPNG(t, color.RGBA{R: 20, G: 40, B: 220, A: 255}))
	t.Logf("纯色对照(noul)\t红图=%.3f\t蓝图=%.3f\t差值=%.3f", red, blue, red-blue)
	if red-blue < 0.3 {
		t.Errorf("图像通路门禁失败：纯色图都不判别（红=%.3f 蓝=%.3f），图像内容未参与判断", red, blue)
	}
}

func solidPNG(t *testing.T, fill color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func probeScoreBytes(t *testing.T, client *jev.Client, name string, question jev.Question, data []byte) float64 {
	t.Helper()
	state := map[string]any{"creative": "image-1", "sourceId": "probe"}
	return decideProbe(t, client, name, question, state, []jev.ImageInput{{Ref: "image-1", Data: data}})
}

// TestLiveRawResponseDump 抓一次完整原始响应，检查 Hub 是否回显 assets 处理情况或告警。
// 复用生产 payload 组装，只在 transport 层旁路复制响应体。
func TestLiveRawResponseDump(t *testing.T) {
	if os.Getenv("AIHUB_API_KEY") == "" {
		t.Skip("未设置 AIHUB_API_KEY，跳过原始响应抓取")
	}
	client := jev.NewFromEnv()
	var captured bytes.Buffer
	client.HTTP = &http.Client{
		Timeout:   120 * time.Second,
		Transport: &captureTransport{inner: http.DefaultTransport, out: &captured},
	}
	question := jev.Question{
		Type:         "noul",
		Instructions: "请只根据 `creative` 中的图片判断画面主色是否为红色。",
		Criteria:     map[string]string{"true": "画面以红色为主", "false": "画面不以红色为主"},
	}
	score := probeScoreBytes(t, client, "red", question, solidPNG(t, color.RGBA{R: 220, G: 20, B: 20, A: 255}))
	body := strings.ReplaceAll(captured.String(), os.Getenv("AIHUB_API_KEY"), "[REDACTED]")
	if len(body) > 3000 {
		body = body[:3000] + "...(截断)"
	}
	t.Logf("红图 noul=%.3f\t原始响应=%s", score, body)
}

type captureTransport struct {
	inner http.RoundTripper
	out   *bytes.Buffer
}

func (c *captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.inner.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	c.out.Write(body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func formatScore(value float64) string {
	if value < 0 {
		return "无分数"
	}
	return fmt.Sprintf("%.3f", value)
}
