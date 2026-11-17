package asset

import (
	"testing"

	"adm_jev/internal/material/model"
)

// TestUnsecuredRequestRuleTable 覆盖无法确定协议和缺少创意地址时的三态，以及追踪像素排除。
func TestUnsecuredRequestRuleTable(t *testing.T) {
	cases := []struct {
		name     string
		assets   []model.Asset
		status   string
		evidence int
		notices  int
	}{
		{
			name: "动态拼接地址无法还原",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "http://cdn.example/${AUCTION_PRICE}/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status: StatusUnknown,
		},
		{
			name: "相对路径无法确定协议",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "/assets/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status: StatusUnknown,
		},
		{
			name: "地址为空",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status: StatusUnknown,
		},
		{
			name: "只有通知地址没有创意地址",
			assets: []model.Asset{
				{SourceID: "notice-1", Kind: "url", Role: "creative", Value: "http://notice.example/win", FieldPath: "seatbid[0].bid[0].nurl"},
			},
			status:  StatusUnknown,
			notices: 1,
		},
		{
			name: "追踪像素使用 HTTP 不命中",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "https://cdn.example/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
				{SourceID: "pixel-1", Kind: "image", Role: "tracker", Value: "http://track.example/p.gif", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status: StatusNotHit,
		},
		{
			name: "创意地址混合协议只记 HTTP 证据",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "https://cdn.example/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
				{SourceID: "img-2", Kind: "image", Role: "creative", Value: "http://cdn.example/b.jpg", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status:   StatusHit,
			evidence: 1,
		},
		{
			name: "创意地址全为 HTTPS",
			assets: []model.Asset{
				{SourceID: "img-1", Kind: "image", Role: "creative", Value: "https://cdn.example/a.jpg", FieldPath: "seatbid[0].bid[0].adm"},
			},
			status: StatusNotHit,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := UnsecuredRequest([]model.BidSource{{Assets: item.assets}})
			if got.Status != item.status || len(got.Evidence) != item.evidence || len(got.Notices) != item.notices {
				t.Fatalf("status=%s evidence=%d notices=%d，期望 %s/%d/%d",
					got.Status, len(got.Evidence), len(got.Notices), item.status, item.evidence, item.notices)
			}
			if item.status == StatusHit && got.Evidence[0].SourceID != "img-2" {
				t.Fatalf("证据来源错误: %+v", got.Evidence)
			}
		})
	}
}
