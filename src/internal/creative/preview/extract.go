package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"adm_jev/internal/creative/model"
)

type rawBid struct {
	path   string
	adm    string
	crid   string
	domain string
	impid  string
	nurl   string
	burl   string
	w      int
	h      int
}

// Extract 从 Bid Response 或裸 ADM 提取每个 bid 的素材来源。
// request 是可选的原始 Bid Request，只用于按 impid 关联 imp.secure。
func Extract(input, request string) ([]model.BidSource, error) {
	input = strings.TrimSpace(input)
	secure := secureByImp(request)
	if bids, ok := responseBids(input); ok {
		out := make([]model.BidSource, 0, len(bids))
		for _, bid := range bids {
			item := extractBid(bid)
			if bid.impid != "" {
				if value, found := secure[bid.impid]; found {
					item.ImpSecure = value
				}
			}
			out = append(out, item)
		}
		return out, nil
	}
	return []model.BidSource{extractBid(rawBid{path: "adm", adm: input})}, nil
}

func responseBids(input string) ([]rawBid, bool) {
	var resp struct {
		Seatbid []struct {
			Bid []struct {
				Adm     string   `json:"adm"`
				Crid    string   `json:"crid"`
				Adomain []string `json:"adomain"`
				W       int      `json:"w"`
				H       int      `json:"h"`
				ImpID   string   `json:"impid"`
				Nurl    string   `json:"nurl"`
				Burl    string   `json:"burl"`
			} `json:"bid"`
		} `json:"seatbid"`
	}
	if err := json.Unmarshal([]byte(input), &resp); err != nil || resp.Seatbid == nil {
		return nil, false
	}
	var bids []rawBid
	for seatIndex, seat := range resp.Seatbid {
		for bidIndex, bid := range seat.Bid {
			item := rawBid{
				path:  fmt.Sprintf("seatbid[%d].bid[%d]", seatIndex, bidIndex),
				adm:   bid.Adm,
				crid:  bid.Crid,
				impid: bid.ImpID,
				nurl:  bid.Nurl,
				burl:  bid.Burl,
				w:     bid.W,
				h:     bid.H,
			}
			if len(bid.Adomain) > 0 {
				item.domain = bid.Adomain[0]
			}
			bids = append(bids, item)
		}
	}
	return bids, true
}

func secureByImp(request string) map[string]*bool {
	request = strings.TrimSpace(request)
	if request == "" {
		return nil
	}
	var body struct {
		Imp []struct {
			ID     string `json:"id"`
			Secure *int   `json:"secure"`
		} `json:"imp"`
	}
	if json.Unmarshal([]byte(request), &body) != nil {
		return nil
	}
	out := map[string]*bool{}
	for _, imp := range body.Imp {
		if imp.ID == "" || imp.Secure == nil {
			continue
		}
		value := *imp.Secure == 1
		out[imp.ID] = &value
	}
	return out
}

type extractor struct {
	meta      bidMeta
	assets    []model.Asset
	index     map[string]int
	admPath   string
	hasScript bool
	hasIframe bool
	hasMraid  bool
}

func extractBid(bid rawBid) model.BidSource {
	item := model.BidSource{
		BidPath:  bid.path,
		Crid:     bid.crid,
		Adomain:  bid.domain,
		W:        bid.w,
		H:        bid.h,
		ImpID:    bid.impid,
		AdmHash:  hashAdm(bid.adm),
		Assets:   []model.Asset{},
		AdmTypes: []string{},
	}
	if strings.TrimSpace(bid.adm) == "" {
		item.ParseStatus = model.ParseFailed
		item.Failure = "ADM 为空"
		return item
	}
	ex := extractor{
		meta:    bidMeta{W: bid.w, H: bid.h},
		index:   map[string]int{},
		admPath: bid.path + ".adm",
	}
	if bid.path == "adm" {
		ex.admPath = "adm"
	}
	doc, err := html.Parse(strings.NewReader(bid.adm))
	if err != nil {
		item.ParseStatus = model.ParseFailed
		item.Failure = "HTML 无法解析"
		return item
	}
	ex.walk(doc)
	var tokenImages []model.Image
	collectCoreImage(bid.adm, &tokenImages)
	for _, image := range tokenImages {
		role := "creative"
		if image.Tracker {
			role = "tracker"
		}
		ex.add("image", image.URL, role)
	}
	lower := strings.ToLower(bid.adm)
	if strings.Contains(lower, "mraid.js") || strings.Contains(lower, "mraid.") {
		ex.hasMraid = true
	}
	ex.add("url", bid.nurl, "creative")
	ex.add("url", bid.burl, "creative")
	if bid.nurl != "" {
		ex.setField(bid.nurl, bid.path+".nurl")
	}
	if bid.burl != "" {
		ex.setField(bid.burl, bid.path+".burl")
	}
	for i := range ex.assets {
		ex.assets[i].SourceID = fmt.Sprintf("%s#%d", bid.path, i)
	}
	item.W = ex.meta.W
	item.H = ex.meta.H
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
				fillSize(&ex.meta, width, height)
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

func hashAdm(adm string) string {
	sum := sha256.Sum256([]byte(adm))
	return hex.EncodeToString(sum[:])
}
