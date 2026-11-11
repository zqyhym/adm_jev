package parser

import (
	"strings"

	"adm_jev/internal/material/model"
)

func parseVideo(bid rawBid) (model.BidSource, bool) {
	if !strings.Contains(strings.ToUpper(bid.adm), "<VAST") {
		return model.BidSource{}, false
	}
	item := bidShell(bid)
	item.ParseStatus = model.ParseUnsupported
	item.Failure = "video 素材提取尚未实现"
	return item, true
}
