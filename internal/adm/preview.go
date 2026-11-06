// Package adm 把 Bid Response 或裸 adm 解析成可预览的素材。
// 监测地址只做改写和列举，不会由服务端发起请求。
package adm

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const emptyURL = "/api/empty"

// Image 是从片段里抽出的一张图或视频地址。
type Image struct {
	URL     string `json:"url"`
	Source  string `json:"source"`
	Tracker bool   `json:"tracker"`
}

// Result 是一次预览解析的结果。
type Result struct {
	Format      string   `json:"format"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	Crid        string   `json:"crid"`
	Adomain     string   `json:"adomain"`
	Images      []Image  `json:"images"`
	Texts       []string `json:"texts"`
	Blocked     []string `json:"blocked"`
	PreviewHTML string   `json:"previewHtml"`
}

type bidMeta struct {
	Adm     string
	Crid    string
	Adomain string
	W       int
	H       int
}

// Parse 接受整段 Bid Response JSON，或裸的 adm 字符串。
func Parse(input string) (Result, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Result{}, fmt.Errorf("内容为空")
	}
	if strings.HasPrefix(input, "{") {
		if meta, ok := parseBidResponse(input); ok {
			return parseAdm(meta.Adm, meta)
		}
		if isNativeJSON(input) {
			return parseNative(input, bidMeta{})
		}
	}
	return parseAdm(input, bidMeta{})
}

func parseBidResponse(input string) (bidMeta, bool) {
	var resp struct {
		Seatbid []struct {
			Bid []struct {
				Adm     string   `json:"adm"`
				Crid    string   `json:"crid"`
				Adomain []string `json:"adomain"`
				W       int      `json:"w"`
				H       int      `json:"h"`
			} `json:"bid"`
		} `json:"seatbid"`
	}
	if err := json.Unmarshal([]byte(input), &resp); err != nil {
		return bidMeta{}, false
	}
	if len(resp.Seatbid) == 0 || len(resp.Seatbid[0].Bid) == 0 {
		return bidMeta{}, false
	}
	bid := resp.Seatbid[0].Bid[0]
	meta := bidMeta{Adm: bid.Adm, Crid: bid.Crid, W: bid.W, H: bid.H}
	if len(bid.Adomain) > 0 {
		meta.Adomain = bid.Adomain[0]
	}
	return meta, true
}

func parseAdm(adm string, meta bidMeta) (Result, error) {
	adm = strings.TrimSpace(adm)
	if adm == "" {
		return Result{}, fmt.Errorf("没有 adm")
	}
	adm = strings.ReplaceAll(adm, "${AUCTION_PRICE}", "AUDIT")
	if looksVAST(adm) {
		return parseVAST(adm, meta), nil
	}
	if strings.HasPrefix(adm, "{") && isNativeJSON(adm) {
		return parseNative(adm, meta)
	}
	return parseBanner(adm, meta), nil
}

func parseBanner(adm string, meta bidMeta) Result {
	var images []Image
	var texts []string
	doc, err := html.Parse(strings.NewReader(adm))
	if err == nil {
		collectHTML(doc, &images, &texts, &meta)
	}
	collectCoreImage(adm, &images)
	images = dedupeImages(images)
	blocked := blockedURLs(images)
	body := rewriteTrackers(adm, blocked)
	body = rewriteMraidSrc(body)
	if meta.W == 0 && meta.H == 0 {
		meta.W, meta.H = 300, 250
	}
	return Result{
		Format:      "banner",
		Width:       meta.W,
		Height:      meta.H,
		Crid:        meta.Crid,
		Adomain:     meta.Adomain,
		Images:      images,
		Texts:       uniqueTexts(texts),
		Blocked:     blocked,
		PreviewHTML: wrapPreview(body),
	}
}

func collectHTML(n *html.Node, images *[]Image, texts *[]string, meta *bidMeta) {
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
		return
	}
	if n.Type == html.TextNode {
		if text := cleanText(n.Data); text != "" {
			*texts = append(*texts, text)
		}
	}
	if n.Type == html.ElementNode {
		if n.Data == "img" {
			src := attr(n, "src")
			style := attr(n, "style")
			width := attr(n, "width")
			height := attr(n, "height")
			tracker := src != "" && isTracker(src, style, width, height)
			if src != "" {
				*images = append(*images, Image{
					URL:     src,
					Source:  "tag",
					Tracker: tracker,
				})
			}
			if !tracker {
				fillSize(meta, width, height)
			}
			for _, srcURL := range srcsetURLs(attr(n, "srcset")) {
				*images = append(*images, Image{URL: srcURL, Source: "srcset", Tracker: trackerURL(srcURL)})
			}
		}
		for _, srcURL := range cssURLs(attr(n, "style")) {
			*images = append(*images, Image{URL: srcURL, Source: "background", Tracker: trackerURL(srcURL)})
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectHTML(c, images, texts, meta)
	}
}

func fillSize(meta *bidMeta, width, height string) {
	if meta.W == 0 {
		if w, err := strconv.Atoi(strings.TrimSuffix(width, "px")); err == nil && w > 1 {
			meta.W = w
		}
	}
	if meta.H == 0 {
		if h, err := strconv.Atoi(strings.TrimSuffix(height, "px")); err == nil && h > 1 {
			meta.H = h
		}
	}
}

var coreImageRes = []*regexp.Regexp{
	regexp.MustCompile(`(?s)"token"\s*:\s*"core_main_image".{0,400}?"value"\s*:\s*"([^"]+)"`),
	regexp.MustCompile(`(?s)\\"token\\"\s*:\s*\\"core_main_image\\".{0,400}?\\"value\\"\s*:\s*\\"([^"\\]+)`),
}

func collectCoreImage(adm string, images *[]Image) {
	for _, re := range coreImageRes {
		m := re.FindStringSubmatch(adm)
		if len(m) < 2 {
			continue
		}
		raw := strings.ReplaceAll(m[1], `\/`, "/")
		unwrapped := unwrapImgsrv(raw)
		*images = append(*images, Image{URL: unwrapped, Source: "token", Tracker: trackerURL(unwrapped)})
		return
	}
}

// unwrapImgsrv 把 Jampp 缩略服务地址还原成后面的 CDN 原图。
func unwrapImgsrv(raw string) string {
	pos := strings.Index(raw, "/imgsrv/")
	if pos < 0 {
		return raw
	}
	rest := raw[pos:]
	for _, marker := range []string{"https%3A", "http%3A", "https://", "http://"} {
		i := strings.Index(rest, marker)
		if i <= 0 {
			continue
		}
		embedded := rest[i:]
		if dec, err := url.PathUnescape(embedded); err == nil && dec != "" {
			return dec
		}
		return embedded
	}
	return raw
}

func dedupeImages(images []Image) []Image {
	index := map[string]int{}
	var out []Image
	for _, img := range images {
		if img.URL == "" {
			continue
		}
		if i, ok := index[img.URL]; ok {
			if img.Tracker {
				out[i].Tracker = true
			}
			continue
		}
		index[img.URL] = len(out)
		out = append(out, img)
	}
	return out
}

func blockedURLs(images []Image) []string {
	var urls []string
	seen := map[string]bool{}
	for _, img := range images {
		if img.Tracker && !seen[img.URL] {
			seen[img.URL] = true
			urls = append(urls, img.URL)
		}
	}
	return urls
}

func rewriteTrackers(adm string, blocked []string) string {
	// 长地址优先，避免短地址先替换后截断长地址。
	ordered := append([]string(nil), blocked...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if len(ordered[j]) > len(ordered[i]) {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	for _, raw := range ordered {
		adm = strings.ReplaceAll(adm, raw, emptyURL)
		escaped := html.EscapeString(raw)
		if escaped != raw {
			adm = strings.ReplaceAll(adm, escaped, emptyURL)
		}
	}
	return adm
}

func rewriteMraidSrc(adm string) string {
	adm = strings.ReplaceAll(adm, `src="mraid.js"`, `src="/mraid.js"`)
	adm = strings.ReplaceAll(adm, `src='mraid.js'`, `src='/mraid.js'`)
	return adm
}

func isTracker(rawURL, style, width, height string) bool {
	if trackerURL(rawURL) {
		return true
	}
	compact := strings.ToLower(strings.ReplaceAll(style, " ", ""))
	if strings.Contains(compact, "left:-9999px") {
		return true
	}
	return tiny(width) && tiny(height) && width != "" && height != ""
}

func tiny(v string) bool {
	v = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(v), "px"))
	return v == "0" || v == "1"
}

func trackerURL(raw string) bool {
	u := strings.ToLower(raw)
	for _, part := range []string{"impression.php", "burl.php", "click.php", "simp.png", "mmp_imp"} {
		if strings.Contains(u, part) {
			return true
		}
	}
	return false
}

func srcsetURLs(srcset string) []string {
	var urls []string
	for _, part := range strings.Split(srcset, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		if strings.HasPrefix(fields[0], "http://") || strings.HasPrefix(fields[0], "https://") {
			urls = append(urls, fields[0])
		}
	}
	return urls
}

var cssURLRe = regexp.MustCompile(`(?i)url\(\s*['"]?(https?://[^)'"]+)`)

func cssURLs(style string) []string {
	var urls []string
	for _, m := range cssURLRe.FindAllStringSubmatch(style, -1) {
		if len(m) > 1 {
			urls = append(urls, m[1])
		}
	}
	return urls
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func cleanText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return ""
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func uniqueTexts(texts []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, text := range texts {
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

func looksVAST(adm string) bool {
	upper := strings.ToUpper(adm)
	return strings.Contains(upper, "<VAST")
}

var (
	mediaFileRe = regexp.MustCompile(`(?is)<MediaFile[^>]*>\s*(?:<!\[CDATA\[)?\s*([^<\s][^<]*?)\s*(?:\]\]>)?\s*</MediaFile>`)
	vastWrapRe  = regexp.MustCompile(`(?is)<VASTAdTagURI[^>]*>\s*(?:<!\[CDATA\[)?\s*([^<\s][^<]*?)\s*(?:\]\]>)?\s*</VASTAdTagURI>`)
	adTitleRe   = regexp.MustCompile(`(?is)<AdTitle[^>]*>\s*(?:<!\[CDATA\[)?\s*([^<]*?)\s*(?:\]\]>)?\s*</AdTitle>`)
)

func parseVAST(adm string, meta bidMeta) Result {
	var images []Image
	var texts []string
	if title := cleanText(firstMatch(adTitleRe, adm)); title != "" {
		texts = append(texts, title)
	}
	for _, m := range mediaFileRe.FindAllStringSubmatch(adm, -1) {
		if len(m) < 2 {
			continue
		}
		media := strings.TrimSpace(m[1])
		images = append(images, Image{URL: media, Source: "media", Tracker: trackerURL(media)})
	}
	images = dedupeImages(images)
	wrapped := vastWrapRe.FindStringSubmatch(adm)
	if len(images) == 0 && len(wrapped) > 1 {
		texts = append(texts, "VAST 包装未展开："+strings.TrimSpace(wrapped[1]))
	}
	blocked := blockedURLs(images)
	if meta.W == 0 && meta.H == 0 {
		meta.W, meta.H = 300, 250
	}
	return Result{
		Format:      "vast",
		Width:       meta.W,
		Height:      meta.H,
		Crid:        meta.Crid,
		Adomain:     meta.Adomain,
		Images:      images,
		Texts:       uniqueTexts(texts),
		Blocked:     blocked,
		PreviewHTML: wrapPreview(vastBody(images, texts)),
	}
}

func vastBody(images []Image, texts []string) string {
	var b strings.Builder
	for _, text := range texts {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(text))
		b.WriteString("</p>")
	}
	for _, img := range images {
		if img.Tracker {
			continue
		}
		src := html.EscapeString(img.URL)
		if isVideoURL(img.URL) {
			b.WriteString(`<video src="`)
			b.WriteString(src)
			b.WriteString(`" controls style="max-width:100%"></video>`)
			continue
		}
		b.WriteString(`<img src="`)
		b.WriteString(src)
		b.WriteString(`" alt="" style="max-width:100%">`)
	}
	if b.Len() == 0 {
		return "<p>没有可预览的媒体文件</p>"
	}
	return b.String()
}

func isVideoURL(raw string) bool {
	u := strings.ToLower(strings.Split(raw, "?")[0])
	return strings.HasSuffix(u, ".mp4") || strings.HasSuffix(u, ".webm") || strings.HasSuffix(u, ".ogg")
}

func firstMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func isNativeJSON(raw string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	if _, ok := payload["assets"]; ok {
		return true
	}
	native, ok := payload["native"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = native["assets"]
	return ok
}

func parseNative(raw string, meta bidMeta) (Result, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Result{}, fmt.Errorf("native 不是合法 JSON")
	}
	assets := nativeAssets(payload)
	var images []Image
	var texts []string
	for _, asset := range assets {
		item, ok := asset.(map[string]any)
		if !ok {
			continue
		}
		if title, ok := item["title"].(map[string]any); ok {
			if text := cleanText(fmt.Sprint(title["text"])); text != "" && text != "<nil>" {
				texts = append(texts, text)
			}
		}
		if data, ok := item["data"].(map[string]any); ok {
			if text := cleanText(fmt.Sprint(data["value"])); text != "" && text != "<nil>" {
				texts = append(texts, text)
			}
		}
		if img, ok := item["img"].(map[string]any); ok {
			src := fmt.Sprint(img["url"])
			if src != "" && src != "<nil>" {
				images = append(images, Image{URL: src, Source: "native", Tracker: trackerURL(src)})
				if meta.W == 0 {
					if w, ok := number(img["w"]); ok {
						meta.W = w
					}
				}
				if meta.H == 0 {
					if h, ok := number(img["h"]); ok {
						meta.H = h
					}
				}
			}
		}
	}
	images = dedupeImages(images)
	blocked := blockedURLs(images)
	if meta.W == 0 && meta.H == 0 {
		meta.W, meta.H = 300, 250
	}
	return Result{
		Format:      "native",
		Width:       meta.W,
		Height:      meta.H,
		Crid:        meta.Crid,
		Adomain:     meta.Adomain,
		Images:      images,
		Texts:       uniqueTexts(texts),
		Blocked:     blocked,
		PreviewHTML: wrapPreview(nativeBody(images, texts)),
	}, nil
}

func nativeAssets(payload map[string]any) []any {
	if assets, ok := payload["assets"].([]any); ok {
		return assets
	}
	native, ok := payload["native"].(map[string]any)
	if !ok {
		return nil
	}
	assets, _ := native["assets"].([]any)
	return assets
}

func number(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func nativeBody(images []Image, texts []string) string {
	var b strings.Builder
	b.WriteString(`<div style="font-family:sans-serif;padding:12px">`)
	for _, img := range images {
		if img.Tracker {
			continue
		}
		b.WriteString(`<img src="`)
		b.WriteString(html.EscapeString(img.URL))
		b.WriteString(`" alt="" style="max-width:100%">`)
	}
	for i, text := range texts {
		tag := "p"
		if i == 0 {
			tag = "h2"
		}
		b.WriteString("<")
		b.WriteString(tag)
		b.WriteString(">")
		b.WriteString(html.EscapeString(text))
		b.WriteString("</")
		b.WriteString(tag)
		b.WriteString(">")
	}
	b.WriteString("</div>")
	return b.String()
}

const guardJS = `(function () {
  var blocked = /impression\.php|burl\.php|click\.php|simp\.png|mmp_imp/i;
  function deny(url) {
    var value = String(url || "");
    if (!blocked.test(value)) return false;
    try { parent.postMessage({ type: "adm-blocked", url: value }, "*"); } catch (e) {}
    return true;
  }
  var desc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, "src");
  if (desc && desc.set && desc.get) {
    Object.defineProperty(HTMLImageElement.prototype, "src", {
      configurable: true,
      get: function () { return desc.get.call(this); },
      set: function (v) {
        if (deny(v)) v = "/api/empty";
        desc.set.call(this, v);
      }
    });
  }
  var rawSet = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function (name, value) {
    if (String(name).toLowerCase() === "src" && deny(value)) value = "/api/empty";
    return rawSet.call(this, name, value);
  };
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      var target = typeof input === "string" ? input : (input && input.url);
      if (deny(target)) return Promise.resolve(new Response("", { status: 204 }));
      return origFetch.apply(this, arguments);
    };
  }
  var open = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url) {
    if (deny(url)) url = "/api/empty";
    var args = [method, url].concat(Array.prototype.slice.call(arguments, 2));
    return open.apply(this, args);
  };
  if (navigator.sendBeacon) {
    var sendBeacon = navigator.sendBeacon.bind(navigator);
    navigator.sendBeacon = function (url, data) {
      if (deny(url)) return true;
      return sendBeacon(url, data);
    };
  }
  window.mraid = window.mraid || {
    getState: function () { return "ready"; },
    getVersion: function () { return "2.0"; },
    isViewable: function () { return true; },
    addEventListener: function (name, cb) {
      if (name === "ready" && typeof cb === "function") setTimeout(cb, 0);
    },
    removeEventListener: function () {},
    open: function (url) { console.log("mraid.open", url); },
    close: function () {}
  };
})();`

func wrapPreview(body string) string {
	return "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><script>" + guardJS + "</script></head><body style=\"margin:0\">" + body + "</body></html>"
}

// MraidJS 是给相对路径 mraid.js 用的同一份桩。
func MraidJS() string {
	return guardJS
}
