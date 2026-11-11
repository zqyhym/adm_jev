package parser

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"adm_jev/internal/material/model"
)

type extractor struct {
	w         int
	h         int
	assets    []model.Asset
	index     map[string]int
	admPath   string
	hasScript bool
	hasIframe bool
	hasMraid  bool
}

func extractBanner(bid rawBid) model.BidSource {
	item := bidShell(bid)
	ex := extractor{
		w:       bid.w,
		h:       bid.h,
		index:   map[string]int{},
		admPath: bid.path + ".adm",
	}
	if bid.path == "adm" {
		ex.admPath = "adm"
	}
	adm := strings.TrimSpace(bid.adm)
	if adm == "" {
		ex.addEndpoints(bid)
		return ex.finish(item, bid)
	}
	if imageURL, ok := bareImageURL(adm); ok {
		role := "creative"
		if trackerURL(imageURL) {
			role = "tracker"
		}
		ex.add("image", imageURL, role)
		ex.addEndpoints(bid)
		return ex.finish(item, bid)
	}
	doc, err := html.Parse(strings.NewReader(bid.adm))
	if err != nil {
		item.ParseStatus = model.ParseFailed
		item.Failure = "HTML 无法解析"
		return item
	}
	ex.walk(doc)
	if raw, ok := coreImage(bid.adm); ok {
		role := "creative"
		if trackerURL(raw) {
			role = "tracker"
		}
		ex.add("image", raw, role)
	}
	lower := strings.ToLower(bid.adm)
	if strings.Contains(lower, "mraid.js") || strings.Contains(lower, "mraid.") {
		ex.hasMraid = true
	}
	ex.addEndpoints(bid)
	return ex.finish(item, bid)
}

func bareImageURL(adm string) (string, bool) {
	if strings.Contains(adm, "<") || strings.ContainsAny(adm, " \t\r\n") {
		return "", false
	}
	parsed, err := url.Parse(adm)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	return adm, true
}

func (ex *extractor) addEndpoints(bid rawBid) {
	ex.add("url", bid.nurl, "creative")
	ex.add("url", bid.burl, "creative")
	ex.add("url", bid.lurl, "creative")
	ex.add("image", bid.iurl, "creative")
	ex.setField(bid.nurl, bid.path+".nurl")
	ex.setField(bid.burl, bid.path+".burl")
	ex.setField(bid.lurl, bid.path+".lurl")
	ex.setField(bid.iurl, bid.path+".iurl")
}

func (ex *extractor) finish(item model.BidSource, bid rawBid) model.BidSource {
	if strings.TrimSpace(bid.adm) == "" && len(ex.assets) == 0 {
		item.ParseStatus = model.ParseFailed
		item.Failure = "ADM 为空"
		return item
	}
	for i := range ex.assets {
		ex.assets[i].SourceID = fmt.Sprintf("%s#%d", bid.path, i)
	}
	item.W = ex.w
	item.H = ex.h
	item.Assets = ex.assets
	item.AdmTypes = ex.types(bid.adm)
	item.ParseStatus, item.Failure = ex.status()
	return item
}

func (ex *extractor) walk(n *html.Node) {
	if n.Type == html.TextNode {
		if text := cleanText(n.Data); text != "" {
			ex.add("text", text, "creative")
		}
		return
	}
	if n.Type == html.ElementNode {
		switch strings.ToLower(n.Data) {
		case "script":
			ex.hasScript = true
			ex.add("script", attr(n, "src"), "dynamic")
			return
		case "style":
			var css strings.Builder
			var write func(*html.Node)
			write = func(node *html.Node) {
				if node.Type == html.TextNode {
					css.WriteString(node.Data)
				}
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					write(child)
				}
			}
			write(n)
			for _, srcURL := range cssURLs(css.String()) {
				if trackerURL(srcURL) {
					ex.add("image", srcURL, "tracker")
				} else {
					ex.add("image", srcURL, "creative")
				}
			}
			return
		case "iframe":
			ex.hasIframe = true
			ex.add("iframe", attr(n, "src"), "dynamic")
		case "img":
			src := attr(n, "src")
			style := attr(n, "style")
			width := attr(n, "width")
			height := attr(n, "height")
			tracker := src != "" && isTracker(src, style, width, height)
			if tracker {
				ex.add("image", src, "tracker")
			} else {
				ex.add("image", src, "creative")
				ex.fillSize(width, height)
			}
			for _, srcURL := range srcsetURLs(attr(n, "srcset")) {
				if tracker || trackerURL(srcURL) {
					ex.add("image", srcURL, "tracker")
				} else {
					ex.add("image", srcURL, "creative")
				}
			}
		case "a":
			ex.add("url", attr(n, "href"), "creative")
		case "link":
			for _, part := range strings.Fields(strings.ToLower(attr(n, "rel"))) {
				if part == "stylesheet" {
					ex.add("url", attr(n, "href"), "dynamic")
					break
				}
			}
		}
		for _, srcURL := range cssURLs(attr(n, "style")) {
			if trackerURL(srcURL) {
				ex.add("image", srcURL, "tracker")
			} else {
				ex.add("image", srcURL, "creative")
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		ex.walk(child)
	}
}

func (ex *extractor) add(kind, value, role string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if kind != "text" {
		if pos, ok := ex.index[value]; ok {
			if role == "tracker" {
				ex.assets[pos].Role = "tracker"
			}
			return
		}
		ex.index[value] = len(ex.assets)
	} else {
		for _, asset := range ex.assets {
			if asset.Kind == "text" && asset.Value == value {
				return
			}
		}
	}
	ex.assets = append(ex.assets, model.Asset{
		Kind:      kind,
		Value:     value,
		FieldPath: ex.admPath,
		Role:      role,
	})
}

func (ex *extractor) setField(value, fieldPath string) {
	value = strings.TrimSpace(value)
	if pos, ok := ex.index[value]; ok {
		ex.assets[pos].FieldPath = fieldPath
	}
}

func (ex *extractor) types(adm string) []string {
	var types []string
	if strings.Contains(adm, "<") {
		types = append(types, "HTML")
	}
	for _, asset := range ex.assets {
		if asset.Kind == "image" {
			types = append(types, "IMAGE")
			break
		}
	}
	if ex.hasScript {
		types = append(types, "SCRIPT")
	}
	if ex.hasIframe {
		types = append(types, "IFRAME")
	}
	if ex.hasMraid {
		types = append(types, "MRAID")
	}
	return types
}

func (ex *extractor) status() (string, string) {
	hasSubject := false
	for _, asset := range ex.assets {
		if asset.Kind == "text" || (asset.Kind == "image" && asset.Role == "creative") {
			hasSubject = true
			break
		}
	}
	if !hasSubject {
		return model.ParsePartial, "缺少已取得的主体素材"
	}
	if ex.hasScript || ex.hasIframe || ex.hasMraid {
		return model.ParsePartial, "存在未执行的动态依赖，素材清单不完整"
	}
	return model.ParseOK, ""
}

func (ex *extractor) fillSize(width, height string) {
	if ex.w == 0 {
		if w, err := strconv.Atoi(strings.TrimSuffix(width, "px")); err == nil && w > 1 {
			ex.w = w
		}
	}
	if ex.h == 0 {
		if h, err := strconv.Atoi(strings.TrimSuffix(height, "px")); err == nil && h > 1 {
			ex.h = h
		}
	}
}

var coreImageRes = []*regexp.Regexp{
	regexp.MustCompile(`(?s)"token"\s*:\s*"core_main_image".{0,400}?"value"\s*:\s*"([^"]+)"`),
	regexp.MustCompile(`(?s)\\"token\\"\s*:\s*\\"core_main_image\\".{0,400}?\\"value\\"\s*:\s*\\"([^"\\]+)`),
}

func coreImage(adm string) (string, bool) {
	for _, re := range coreImageRes {
		m := re.FindStringSubmatch(adm)
		if len(m) < 2 {
			continue
		}
		raw := strings.ReplaceAll(m[1], `\/`, "/")
		pos := strings.Index(raw, "/imgsrv/")
		if pos < 0 {
			return raw, true
		}
		rest := raw[pos:]
		for _, marker := range []string{"https%3A", "http%3A", "https://", "http://"} {
			i := strings.Index(rest, marker)
			if i <= 0 {
				continue
			}
			embedded := rest[i:]
			if dec, err := url.PathUnescape(embedded); err == nil && dec != "" {
				return dec, true
			}
			return embedded, true
		}
		return raw, true
	}
	return "", false
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
