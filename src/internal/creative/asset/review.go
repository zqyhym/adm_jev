package asset

import (
	"context"
	"errors"
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
			questions := map[string]jev.Question{}
			for _, label := range visualLabels {
				questions[label.Name] = jev.Question{
					Type:         "noul",
					Instructions: "请只根据 `creative` 中的图片和同条请求中的文案判断是否属于 " + label.Name + "。",
					Criteria:     map[string]string{"true": label.Hit, "false": label.Exclude},
				}
			}
			requests = append(requests, jev.Request{
				State:             state,
				Questions:         questions,
				Images:            []jev.ImageInput{{Ref: ref, Data: data}},
				EnableImageHeader: true,
			})
		}
	}
	return requests
}

// LabelResult 是一张图片上的一个视觉标签。Probability 在本阶段保持为空。
type LabelResult struct {
	Label       string
	SourceID    string
	Status      string
	Noul        *float64
	Probability *float64
}

const (
	StatusScored  = "SCORED"
	StatusUnknown = "UNKNOWN"
	StatusHit     = "HIT"
	StatusNotHit  = "NOT_HIT"
)

type visualLabel struct {
	Name    string
	Hit     string
	Exclude string
}

var visualLabels = []visualLabel{
	{"Dating", "约会、相亲、征友、婚恋、匹配服务", "情侣或婚礼内容本身不是约会服务"},
	{"Gambling", "赌场、下注、彩票、竞猜、赢钱号召", "普通游戏和无赢钱承诺的虚拟金币"},
	{"Nudity", "可见裸体，或裸露胸部、臀部、生殖器", "普通暴露服装、泳装本身"},
	{"Sexually Suggestive", "未裸体，但姿势、服装、镜头或文案有明显性暗示", "普通泳装、健身且无性暗示"},
	{"Marijuana", "明确推广大麻、THC、相关药房、吸食或制品", "只有叶子图案、没有相关文案或符号"},
	{"Adult Products", "性玩具、情趣用品", "普通药品"},
	{"Adult Entertainment", "成人影像、色情站点、色情直播或陪聊", "只销售成人用品、没有成人内容或陪聊"},
	{"Profane Language", "文案或画面文字含脏话、侮辱性粗口", "有性暗示但没有脏话"},
	{"Religion Spirituality", "宗教、信仰、灵修、占卜、塔罗，或寺庙、教堂相关传播和劝募", "文化符号或建筑偶然出现、没有信仰号召"},
	{"Violence", "流血、伤口、殴打、枪击、刀刺、爆炸伤人或尸体", "单独的枪械图标、普通体育对抗"},
}

// Submit 在有主体图片时调用 Hub，并按来源返回 10 类标签。没有可送审图片时不发起请求，标签为 UNKNOWN。
func Submit(ctx context.Context, client *jev.Client, bids []model.BidSource, files map[string][]byte) ([]LabelResult, error) {
	requests := Reviews(bids, files)
	if len(requests) == 0 {
		return unknownForUnsent(bids), fmt.Errorf("没有可送审的主体图片")
	}
	var labels []LabelResult
	for _, request := range requests {
		sourceID, _ := request.State.(map[string]any)["sourceId"].(string)
		result, err := client.Decide(ctx, request)
		if result == nil {
			labels = append(labels, unknownLabels(sourceID)...)
			return labels, err
		}
		for _, label := range visualLabels {
			answer := result.Answers[label.Name]
			if answer.Status != jev.StatusAnswered || answer.Noul == nil || *answer.Noul < 0 || *answer.Noul > 1 {
				labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusUnknown})
				continue
			}
			score := *answer.Noul
			labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusScored, Noul: &score})
		}
		var missing *jev.MissingAnswersError
		if err != nil && !errors.As(err, &missing) {
			return labels, err
		}
	}
	return labels, nil
}

func unknownForUnsent(bids []model.BidSource) []LabelResult {
	var labels []LabelResult
	for _, bid := range bids {
		for _, asset := range bid.Assets {
			if asset.Kind == "image" && asset.Role == "creative" {
				labels = append(labels, unknownLabels(asset.SourceID)...)
			}
		}
	}
	if len(labels) == 0 {
		labels = unknownLabels("")
	}
	return labels
}

func unknownLabels(sourceID string) []LabelResult {
	labels := make([]LabelResult, 0, len(visualLabels))
	for _, label := range visualLabels {
		labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusUnknown})
	}
	return labels
}
