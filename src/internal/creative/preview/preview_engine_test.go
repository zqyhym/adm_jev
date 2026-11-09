package preview

import (
	"strings"
	"testing"

	"adm_jev/internal/creative/model"
)

func TestStaticImage(t *testing.T) {
	adm := `<a href="https://example.com/land"><img src="https://cdn.example/a.jpg" width="320" height="50" alt="tire"></a><div>秋季促销</div>`
	got, err := Parse(adm)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "banner" {
		t.Fatalf("format=%s", got.Format)
	}
	if got.Width != 320 || got.Height != 50 {
		t.Fatalf("size=%dx%d", got.Width, got.Height)
	}
	if len(got.Images) != 1 || got.Images[0].URL != "https://cdn.example/a.jpg" || got.Images[0].Tracker {
		t.Fatalf("images=%+v", got.Images)
	}
	if len(got.Texts) != 1 || got.Texts[0] != "秋季促销" {
		t.Fatalf("texts=%v", got.Texts)
	}
	if len(got.Blocked) != 0 {
		t.Fatalf("blocked=%v", got.Blocked)
	}
}

func TestTrackingPixel(t *testing.T) {
	adm := `<img src='https://gw.example/impression.php?a=1&amp;b=2' style='position:absolute;left:-9999px'/><img src="https://imp.example/mmp_imp/v2/xx" width="1" height="1"/>`
	got, err := Parse(adm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 2 {
		t.Fatalf("images=%+v", got.Images)
	}
	for _, img := range got.Images {
		if !img.Tracker {
			t.Fatalf("expected tracker: %+v", img)
		}
	}
	if strings.Contains(got.PreviewHTML, "gw.example/impression.php") || strings.Contains(got.PreviewHTML, "imp.example/mmp_imp") {
		t.Fatal("preview still contains tracker url")
	}
	if !strings.Contains(got.PreviewHTML, "/api/empty") {
		t.Fatal("tracker was not rewritten")
	}
}

func TestJamppToken(t *testing.T) {
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
	got, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Crid != "6868058" || got.Adomain != "autodoc.de" || got.Width != 300 || got.Height != 250 {
		t.Fatalf("meta=%+v", got)
	}
	var creative *model.Image
	for i := range got.Images {
		if got.Images[i].Source == "token" {
			creative = &got.Images[i]
		}
	}
	if creative == nil {
		t.Fatalf("images=%+v", got.Images)
	}
	const want = "https://cdn.jampp.com/assets/1626/Purple221-1781688235340.jpg"
	if creative.URL != want || creative.Tracker {
		t.Fatalf("token image=%+v", creative)
	}
	if strings.Contains(got.PreviewHTML, "gw.example/impression.php") {
		t.Fatal("preview requested impression.php")
	}
	if !strings.Contains(got.PreviewHTML, "/mraid.js") {
		t.Fatal("mraid.js was not rewritten")
	}
}

func TestNativeAndVast(t *testing.T) {
	native := `{"native":{"assets":[{"title":{"text":"配件促销"}},{"img":{"url":"https://cdn.example/n.jpg","w":300,"h":250}},{"data":{"value":"立即查看"}}]}}`
	got, err := Parse(native)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "native" || len(got.Images) != 1 || got.Texts[0] != "配件促销" {
		t.Fatalf("native=%+v", got)
	}
	if strings.Contains(got.PreviewHTML, "<script") && strings.Contains(got.PreviewHTML, "native") {
		t.Fatal("native preview should not embed the original payload")
	}

	vast := `<VAST version="3.0"><Ad><Wrapper><VASTAdTagURI><![CDATA[https://dsp.example/vast.xml]]></VASTAdTagURI></Wrapper></Ad></VAST>`
	got, err = Parse(vast)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "vast" || len(got.Images) != 0 || !strings.Contains(got.Texts[0], "未展开") {
		t.Fatalf("vast=%+v", got)
	}
	if strings.Contains(got.PreviewHTML, "https://dsp.example/vast.xml") && strings.Contains(got.PreviewHTML, "<video") {
		t.Fatal("wrapper must not be requested as media")
	}
}
