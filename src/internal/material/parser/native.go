package parser

import (
	"encoding/json"
	"strings"

	"adm_jev/internal/material/model"
)

func parseNative(bid rawBid) (model.BidSource, bool) {
	if !isNativeJSON(strings.TrimSpace(bid.adm)) {
		return model.BidSource{}, false
	}
	item := bidShell(bid)
	item.ParseStatus = model.ParseUnsupported
	item.Failure = "native 素材提取尚未实现"
	return item, true
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
