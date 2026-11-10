package asset

import (
	"context"
	"fmt"
	"strings"

	"adm_jev/internal/creative/model"
	"adm_jev/internal/jev"
)

// Reviews 为每张已取得的主体图片组装一条决策请求。没有可送审图片时返回空。
func Reviews(bids []model.BidSource, files map[string][]byte) []jev.Request {
	var requests []jev.Request
	for _, bid := range bids {
		var texts []string
		for _, asset := range bid.Assets {
			if asset.Kind == "text" && asset.Role == "creative" {
				texts = append(texts, asset.Value)
			}
		}
		for _, asset := range bid.Assets {
			data := files[asset.SourceID]
			if asset.Kind != "image" || asset.Role != "creative" || !asset.Obtained || len(data) == 0 {
				continue
			}
			ref := "asset-" + strings.NewReplacer("[", "", "]", "", "#", "-").Replace(asset.SourceID)
			state := map[string]any{"creative": ref, "sourceId": asset.SourceID}
			if len(texts) > 0 {
				state["copy"] = strings.Join(texts, "\n")
			}
			requests = append(requests, jev.Request{
				State: state,
				Questions: map[string]jev.Question{
					"review": {
						Type:         "noul",
						Instructions: "请只根据 `creative` 中的图片判断画面是否可辨认。",
						Criteria:     map[string]string{"true": "图片内容可辨认", "false": "图片无法辨认"},
					},
				},
				Images:            []jev.ImageInput{{Ref: ref, Data: data}},
				EnableImageHeader: true,
			})
		}
	}
	return requests
}

// Submit 在有主体图片时调用 Hub。没有可送审图片时直接返回，不发起请求。
func Submit(ctx context.Context, client *jev.Client, bids []model.BidSource, files map[string][]byte) error {
	requests := Reviews(bids, files)
	if len(requests) == 0 {
		return fmt.Errorf("没有可送审的主体图片")
	}
	for _, request := range requests {
		if _, err := client.Decide(ctx, request); err != nil {
			return err
		}
	}
	return nil
}
