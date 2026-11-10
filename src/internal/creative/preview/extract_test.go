package preview

import (
	"strings"
	"testing"

	"adm_jev/internal/creative/model"
)

// TestExtractStaticImage 验证静态图片、落地页和可见文案能解析为 OK，并保留尺寸与来源。
func TestExtractStaticImage(t *testing.T) {
	const adm = `<a href="https://example.com/land"><img src="https://cdn.example/a.jpg" width="320" height="50" alt="tire"></a><div>秋季促销</div>`
	got := one(t, adm, "")
	if got.BidPath != "adm" || got.ParseStatus != model.ParseOK || got.Failure != "" {
		t.Fatalf("%+v", got)
	}
	if got.W != 320 || got.H != 50 || got.AdmHash != hashAdm(adm) {
		t.Fatalf("size=%dx%d hash=%s", got.W, got.H, got.AdmHash)
	}
	if !hasType(got, "HTML") || !hasType(got, "IMAGE") {
		t.Fatalf("types=%v", got.AdmTypes)
	}
	image := findAsset(t, got, "image", "https://cdn.example/a.jpg")
	text := findAsset(t, got, "text", "秋季促销")
	link := findAsset(t, got, "url", "https://example.com/land")
	for _, asset := range []model.Asset{image, text, link} {
		if asset.Obtained || asset.FieldPath != "adm" || !strings.HasPrefix(asset.SourceID, "adm#") {
			t.Fatalf("%+v", asset)
		}
	}
	if image.Role != "creative" {
		t.Fatalf("image=%+v", image)
	}
}

// TestExtractTrackers 验证只有追踪像素时状态为 PARTIAL，且不会当成主体素材。
func TestExtractTrackers(t *testing.T) {
	adm := `<img src='https://gw.example/impression.php?a=1&amp;b=2' style='position:absolute;left:-9999px'/><img src="https://imp.example/mmp_imp/v2/xx" width="1" height="1"/>`
	got := one(t, adm, "")
	if got.ParseStatus != model.ParsePartial || got.Failure != "缺少已取得的主体素材" {
		t.Fatalf("%+v", got)
	}
	var trackers int
	for _, asset := range got.Assets {
		if asset.Kind == "image" {
			trackers++
			if asset.Role != "tracker" || asset.Obtained {
				t.Fatalf("%+v", asset)
			}
		}
	}
	if trackers != 2 {
		t.Fatalf("assets=%+v", got.Assets)
	}
}

// TestExtractJampp 验证 Jampp 主图、曝光像素和 MRAID 脚本同时存在时状态为 PARTIAL。
func TestExtractJampp(t *testing.T) {
	const input = `{
  "id": "demo",
  "seatbid": [{
    "bid": [{
      "crid": "6868058",
      "adomain": ["autodoc.de"],
      "w": 300,
      "h": 250,
      "adm": "<img src='https://gw.example/impression.php?id=1' style='position:absolute;left:-9999px'/><script src=\"mraid.js\"></script><script>var jmptokens = [{\"token\":\"core_main_image\",\"type\":\"token\",\"value\":\"https://imgx.jampp.com/imgsrv/tn/mx150/auto/135/135/https%3A//cdn.jampp.com/assets/1626/Purple221-1781688235340.jpg\"}];</script>"
    }]
  }]
}`
	got := one(t, input, "")
	if got.BidPath != "seatbid[0].bid[0]" || got.Crid != "6868058" || got.Adomain != "autodoc.de" || got.W != 300 || got.H != 250 {
		t.Fatalf("%+v", got)
	}
	if got.ParseStatus != model.ParsePartial || !strings.Contains(got.Failure, "动态依赖") {
		t.Fatalf("%+v", got)
	}
	creative := findAsset(t, got, "image", "https://cdn.jampp.com/assets/1626/Purple221-1781688235340.jpg")
	if creative.Role != "creative" || creative.Obtained || creative.FieldPath != "seatbid[0].bid[0].adm" {
		t.Fatalf("%+v", creative)
	}
	tracker := findAsset(t, got, "image", "https://gw.example/impression.php?id=1")
	if tracker.Role != "tracker" {
		t.Fatalf("%+v", tracker)
	}
	script := findAsset(t, got, "script", "mraid.js")
	if script.Role != "dynamic" {
		t.Fatalf("%+v", script)
	}
	for _, kind := range []string{"HTML", "IMAGE", "SCRIPT", "MRAID"} {
		if !hasType(got, kind) {
			t.Fatalf("types=%v", got.AdmTypes)
		}
	}
}

// TestExtractMultipleBids 验证同一响应里的多个 bid 分开提取，图片不会串到别的 bid。
func TestExtractMultipleBids(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<img src=\"https://cdn.example/a.jpg\">"},{"adm":"<img src=\"https://cdn.example/b.jpg\">"}]}]}`
	got, err := Extract(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].BidPath != "seatbid[0].bid[0]" || got[1].BidPath != "seatbid[0].bid[1]" {
		t.Fatalf("%+v", got)
	}
	findAsset(t, got[0], "image", "https://cdn.example/a.jpg")
	findAsset(t, got[1], "image", "https://cdn.example/b.jpg")
	if strings.Contains(assetValues(got[0]), "b.jpg") || strings.Contains(assetValues(got[1]), "a.jpg") {
		t.Fatalf("%+v", got)
	}
}

// TestExtractIframe 验证只有 Iframe 时记录地址并标为 PARTIAL，不把框架内容当成已审核素材。
func TestExtractIframe(t *testing.T) {
	got := one(t, `<iframe src="https://ads.example/frame.html"></iframe>`, "")
	frame := findAsset(t, got, "iframe", "https://ads.example/frame.html")
	if frame.Role != "dynamic" || frame.Obtained || got.ParseStatus != model.ParsePartial {
		t.Fatalf("%+v %+v", got, frame)
	}
	if !hasType(got, "IFRAME") {
		t.Fatalf("types=%v", got.AdmTypes)
	}
}

// TestExtractEmptyAdm 验证空 ADM 为 FAILED，不产生无违规结论。
func TestExtractEmptyAdm(t *testing.T) {
	got := one(t, "  ", "")
	if got.ParseStatus != model.ParseFailed || len(got.Assets) != 0 || strings.Contains(got.Failure, "无违规") {
		t.Fatalf("%+v", got)
	}
	const input = `{"seatbid":[{"bid":[{"crid":"empty","adm":""},{"adm":"<img src=\"https://cdn.example/a.jpg\">"}]}]}`
	all, err := Extract(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ParseStatus != model.ParseFailed || all[0].Crid != "empty" || len(all[0].Assets) != 0 {
		t.Fatalf("%+v", all[0])
	}
	if all[1].ParseStatus != model.ParseOK {
		t.Fatalf("%+v", all[1])
	}
}

// TestExtractMissingSize 验证响应和标签都没有尺寸时宽高保持 0。
func TestExtractMissingSize(t *testing.T) {
	got := one(t, `<img src="https://cdn.example/a.jpg"><div>秋季促销</div>`, "")
	if got.W != 0 || got.H != 0 || got.ParseStatus != model.ParseOK {
		t.Fatalf("%+v", got)
	}
}

// TestExtractImpSecure 验证只有请求能按 impid 匹配时才写入 imp.secure。
func TestExtractImpSecure(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"impid":"imp-1","adm":"<img src=\"https://cdn.example/a.jpg\" width=\"320\" height=\"50\">"}]}]}`
	const request = `{"imp":[{"id":"imp-1","secure":1},{"id":"other","secure":0}]}`
	got := one(t, input, request)
	if got.ImpID != "imp-1" || got.ImpSecure == nil || !*got.ImpSecure {
		t.Fatalf("%+v", got)
	}
	missing := one(t, input, "")
	if missing.ImpSecure != nil {
		t.Fatalf("%+v", missing)
	}
}

// TestExtractNoticeURL 验证 nurl 记为 URL 来源，不判断协议是否违规。
func TestExtractNoticeURL(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"nurl":"http://notice.example/win","adm":"<img src=\"https://cdn.example/a.jpg\" width=\"320\" height=\"50\">"}]}]}`
	got := one(t, input, "")
	notice := findAsset(t, got, "url", "http://notice.example/win")
	if notice.FieldPath != "seatbid[0].bid[0].nurl" || notice.Obtained {
		t.Fatalf("%+v", notice)
	}
	if got.ParseStatus != model.ParseOK || strings.Contains(got.Failure, "违规") {
		t.Fatalf("%+v", got)
	}
}

// TestExtractIURL 验证 iurl 记为未下载的创意图片，字段路径指向 iurl。
func TestExtractIURL(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<div></div>","iurl":"https://cdn.example/preview.png"}]}]}`
	got := one(t, input, "")
	image := findAsset(t, got, "image", "https://cdn.example/preview.png")
	if image.Role != "creative" || image.Obtained || image.FieldPath != "seatbid[0].bid[0].iurl" {
		t.Fatalf("%+v", image)
	}
}

// TestExtractLURL 验证 lurl 只记为通知地址，不作为图片来源。
func TestExtractLURL(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"adm":"<img src=\"https://cdn.example/a.jpg\" width=\"320\" height=\"50\">","lurl":"https://rtbnotify.example/loss"}]}]}`
	got := one(t, input, "")
	notice := findAsset(t, got, "url", "https://rtbnotify.example/loss")
	if notice.FieldPath != "seatbid[0].bid[0].lurl" {
		t.Fatalf("%+v", notice)
	}
	for _, asset := range got.Assets {
		if asset.Kind == "image" && asset.Value == notice.Value {
			t.Fatalf("lurl 被当成图片: %+v", asset)
		}
	}
}

// TestExtractLiftoffShape 验证预览图会被列出，曝光像素仍是 tracker，动态依赖保持 PARTIAL。
func TestExtractLiftoffShape(t *testing.T) {
	const input = `{"seatbid":[{"bid":[{"w":320,"h":480,"adm":"<img src='https://tracker.zmaticoo.com/impression.php?id=1' style='position:absolute;left:-9999px'/><script src='mraid.js'></script>","iurl":"https://cdn.liftoff-creatives.io/customers/417/previews/15c1d3a623-ios-f8f3e8804a.png"}]}]}`
	got := one(t, input, "")
	preview := findAsset(t, got, "image", "https://cdn.liftoff-creatives.io/customers/417/previews/15c1d3a623-ios-f8f3e8804a.png")
	if preview.Role != "creative" || preview.Obtained || !strings.HasSuffix(preview.FieldPath, ".iurl") {
		t.Fatalf("%+v", preview)
	}
	tracker := findAsset(t, got, "image", "https://tracker.zmaticoo.com/impression.php?id=1")
	if tracker.Role != "tracker" || got.ParseStatus != model.ParsePartial {
		t.Fatalf("tracker=%+v status=%s", tracker, got.ParseStatus)
	}
}

func one(t *testing.T, input, request string) model.BidSource {
	t.Helper()
	got, err := Extract(input, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	return got[0]
}

func findAsset(t *testing.T, got model.BidSource, kind, value string) model.Asset {
	t.Helper()
	for _, asset := range got.Assets {
		if asset.Kind == kind && asset.Value == value {
			return asset
		}
	}
	t.Fatalf("missing %s %s in %+v", kind, value, got.Assets)
	return model.Asset{}
}

func hasType(got model.BidSource, kind string) bool {
	for _, item := range got.AdmTypes {
		if item == kind {
			return true
		}
	}
	return false
}

func assetValues(got model.BidSource) string {
	var values []string
	for _, asset := range got.Assets {
		values = append(values, asset.Value)
	}
	return strings.Join(values, " ")
}
