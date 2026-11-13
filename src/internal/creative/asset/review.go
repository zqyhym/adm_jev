package asset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"adm_jev/internal/jev"
	"adm_jev/internal/material/model"
)

// LabelResult 是一张图片上的一个视觉标签。Probability 在本阶段保持为空。
// Reason 只在 Status 为 UNKNOWN 时填写，说明图片未送审或送审后未取得分数的原因。
type LabelResult struct {
	Label       string
	SourceID    string
	Status      string
	Reason      string
	Noul        *float64
	Probability *float64
}

const (
	StatusScored  = "SCORED"
	StatusUnknown = "UNKNOWN"
	StatusHit     = "HIT"
	StatusNotHit  = "NOT_HIT"
)

// 素材层原因：图片没有进入决策请求。
const (
	ReasonNotObtained  = "NOT_OBTAINED"
	ReasonNoData       = "NO_DATA"
	ReasonImageSize    = "IMAGE_SIZE"
	ReasonInvalidImage = "INVALID_IMAGE"
	ReasonNoSubject    = "NO_SUBJECT"
)

// 模型层原因：图片已送审但没有取得有效分数。
const (
	ReasonHubFailed     = "HUB_FAILED"
	ReasonMissingAnswer = "MISSING_ANSWER"
)

type visualLabel struct {
	Name    string
	Hit     string
	Exclude string
}

// 素材违规标签
var visualLabels = []visualLabel{
	{"Dating",
		"推广约会、相亲、征友、婚恋或匹配服务：交友/匹配类 App 界面、'meet singles/找对象' 类号召、引导注册寻找伴侣",
		"情侣照片、婚纱摄影、婚礼服务本身；不以婚恋匹配为目的的普通社交 App；恋爱题材剧情但不推广交友服务"},
	{"Gambling",
		"推广赌场、体育博彩、彩票、竞猜或真金赌博：充值提现、赔率展示、赢钱号召、博彩 App 界面",
		"普通电子游戏；不可兑现且无赢钱承诺的虚拟金币/积分（含社交博彩）；普通游戏开箱；无彩头的竞技竞猜"},
	{"Nudity",
		"可见裸体：裸露生殖器、臀部、女性乳房/乳头、全裸人体，或明显透视装",
		"泳装、内衣、深 V 服装本身（若性暗示归 Sexually Suggestive）；哺乳、医疗、艺术语境的非性化裸露"},
	{"Sexually Suggestive",
		"未裸体但有明显性暗示：性暗示姿势或表情、刻意突出性征的服装或镜头、性暗示文案或双关",
		"无性暗示的普通泳装、健身、舞蹈；无暗示姿势或镜头的内衣普通展示；拥抱、亲吻等非性化亲密"},
	{"Marijuana",
		"推广大麻、THC 制品、相关药房、吸食方式或制品：大麻叶配推广文案、药房广告、吸食描绘",
		"只有叶子图案、没有相关文案或符号；明确标注无精神活性的 CBD/汉麻制品"},
	{"Adult Products",
		"展示或推广性玩具、情趣用品及其销售",
		"普通药品、医疗器械、保健品；普通内衣（若性暗示归 Sexually Suggestive）"},
	{"Adult Entertainment",
		"推广成人影像、色情站点、色情直播、付费陪聊等成人内容服务：色情站点标识/域名、成人内容预览、付费陪聊号召",
		"只销售成人用品（归 Adult Products）；非色情直播、视频平台或普通社交陪聊"},
	{"Profane Language",
		"文案或画面文字含脏话、侮辱性粗口或歧视性詈词，包括 f*ck、sh*t 等遮罩写法",
		"无侮辱意图的轻度感叹或口头禅；有性暗示但没有脏话"},
	{"Religion Spirituality",
		"推广宗教、信仰、灵修、占卜、塔罗、占星服务，或寺庙、教堂的传播劝募：信仰号召、付费占卜、劝捐",
		"文化符号、宗教建筑偶然出现且无信仰号召；节日文化内容；无灵修推广的普通瑜伽/冥想健身"},
	{"Violence",
		"写实暴力或伤害：流血、伤口、殴打、枪击、刀刺、爆炸伤人、尸体、写实虐待描绘",
		"无写实流血伤害的游戏/动漫风格化战斗；单独枪械图标或展示且无暴力行为；普通体育对抗"},
}

// Reviews 为每张已取得的主体图片组装一条决策请求。没有可送审图片时返回空。
func Reviews(bids []model.BidSource, files map[string][]byte) []jev.Request {
	var requests []jev.Request
	for _, bid := range bids {
		//是否有文案
		var texts []string
		for _, asset := range bid.Assets {
			if asset.Kind == "text" && asset.Role == "creative" {
				texts = append(texts, asset.Value)
			}
		}
		for _, asset := range bid.Assets {
			if !isSubjectImage(asset) || imageSkipReason(asset, files) != "" {
				continue
			}
			data := files[asset.SourceID]
			ref := "asset-" + strings.NewReplacer("[", "", "]", "", "#", "-").Replace(asset.SourceID)
			//请求中的每个问题（question）都基于同一个 state。
			state := map[string]any{"creative": ref, "sourceId": asset.SourceID}
			//加入文案
			if len(texts) > 0 {
				state["copy"] = strings.Join(texts, "\n")
			}
			//加入10个素材违规标签问题 noul
			questions := map[string]jev.Question{}
			for _, label := range visualLabels {
				questions[label.Name] = jev.Question{
					Type:         "noul",
					Instructions: "请只根据 `creative` 中的图片和同条请求中的文案判断是否属于 " + label.Name + "。",
					Criteria:     map[string]string{"true": label.Hit, "false": label.Exclude},
				}
			}

			//封装为jev标准请求
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

// Submit 在有主体图片时调用 Hub，并按来源返回 10 类标签。
// 每张主体图片都有 10 行结果：未送审或未取得有效分数的为 UNKNOWN，并带上 Reason。
func Submit(ctx context.Context, client *jev.Client, bids []model.BidSource, files map[string][]byte) ([]LabelResult, error) {
	requests := Reviews(bids, files)
	sent := map[string]bool{}
	for _, request := range requests {
		sourceID, _ := request.State.(map[string]any)["sourceId"].(string)
		sent[sourceID] = true
	}
	var labels []LabelResult
	for _, bid := range bids {
		for _, item := range bid.Assets {
			if !isSubjectImage(item) || sent[item.SourceID] {
				continue
			}
			labels = append(labels, unknownLabels(item.SourceID, imageSkipReason(item, files))...)
		}
	}
	if len(requests) == 0 {
		if len(labels) == 0 {
			labels = unknownLabels("", ReasonNoSubject)
		}
		return labels, fmt.Errorf("没有可送审的主体图片")
	}
	for _, request := range requests {
		sourceID, _ := request.State.(map[string]any)["sourceId"].(string)

		//发送到jev审核
		result, err := client.Decide(ctx, request)
		if result == nil {
			labels = append(labels, unknownLabels(sourceID, ReasonHubFailed)...)
			return labels, err
		}
		for _, label := range visualLabels {
			answer := result.Answers[label.Name]
			if answer.Status != jev.StatusAnswered || answer.Noul == nil || *answer.Noul < 0 || *answer.Noul > 1 {
				labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusUnknown, Reason: ReasonMissingAnswer})
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

func isSubjectImage(item model.Asset) bool {
	return item.Kind == "image" && item.Role == "creative"
}

// imageSkipReason 返回主体图片不能送审的原因，空串表示可送审。Reviews 与 Submit 共用同一判定。
func imageSkipReason(item model.Asset, files map[string][]byte) string {
	if !item.Obtained {
		return ReasonNotObtained
	}
	data := files[item.SourceID]
	if len(data) == 0 {
		return ReasonNoData
	}
	err := jev.ValidateImage(data)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, jev.ErrImageSize):
		return ReasonImageSize
	default:
		return ReasonInvalidImage
	}
}

func unknownLabels(sourceID, reason string) []LabelResult {
	labels := make([]LabelResult, 0, len(visualLabels))
	for _, label := range visualLabels {
		labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusUnknown, Reason: reason})
	}
	return labels
}
