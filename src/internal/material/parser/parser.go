package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"adm_jev/internal/material/model"
)

type rawBid struct {
	path   string
	adm    string
	crid   string
	domain string
	impid  string
	nurl   string
	burl   string
	iurl   string
	lurl   string
	w      int
	h      int
}

// Parse 从 Bid Response 或裸 ADM 提取每个 bid 的素材来源。
func Parse(input string) ([]model.BidSource, error) {
	input = strings.TrimSpace(input)
	if bids, ok := responseBids(input); ok {
		out := make([]model.BidSource, 0, len(bids))
		for _, bid := range bids {
			out = append(out, parseBid(bid))
		}
		return out, nil
	}
	return []model.BidSource{parseBid(rawBid{path: "adm", adm: input})}, nil
}

func parseBid(bid rawBid) model.BidSource {
	if item, ok := parseVideo(bid); ok {
		return item
	}
	if item, ok := parseNative(bid); ok {
		return item
	}
	return extractBanner(bid)
}

func bidShell(bid rawBid) model.BidSource {
	return model.BidSource{
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
				IURL    string   `json:"iurl"`
				LURL    string   `json:"lurl"`
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
				iurl:  bid.IURL,
				lurl:  bid.LURL,
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

func hashAdm(adm string) string {
	sum := sha256.Sum256([]byte(adm))
	return hex.EncodeToString(sum[:])
}
