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
// UpstreamModel 是 Hub 回显的实际上游模型；模型未应答（Hub 失败、素材未送审）时为空。
type LabelResult struct {
	Label         string
	SourceID      string
	Status        string
	Reason        string
	UpstreamModel string
	Noul          *float64
	Probability   *float64
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

// 素材敏感类目标签，口径对齐 TMT：内容属于该敏感垂类即命中，不要求推广意图。
var visualLabels = []visualLabel{
	{"Dating",
		"约会、相亲、征友、婚恋或匹配服务内容：交友/匹配/hookup 类 App 界面、'meet singles/找对象' 类号召、引导注册寻找伴侣",
		"情侣照片、婚纱摄影、婚礼服务本身；不以婚恋匹配为目的的普通社交 App；恋爱题材剧情但不涉及交友服务"},
	{"Gambling",
		"赌场、体育博彩、彩票、竞猜或社交博彩内容：老虎机/扑克/宾果等 casino 玩法（含不可兑现虚拟金币）、充值提现、赔率展示、赢钱号召、博彩 App 界面",
		"无 casino/博彩玩法的普通电子游戏；无彩头且无博彩机制的竞技竞猜"},
	{"Nudity",
		"可见裸体：裸露生殖器、臀部、女性乳房/乳头、全裸人体，或明显透视装",
		"泳装、内衣、深 V 服装本身（若性暗示归 Sexually Suggestive）；哺乳、医疗、艺术语境的非性化裸露"},
	{"Sexually Suggestive",
		"未裸体但有明显性暗示：性暗示姿势或表情、刻意突出性征的服装或镜头、性暗示文案或双关",
		"无性暗示的普通泳装、健身、舞蹈；无暗示姿势或镜头的内衣普通展示；拥抱、亲吻等非性化亲密"},
	{"Marijuana",
		"大麻/THC 相关内容：大麻叶图案或符号（即使无推广文案）、吸食描绘、相关药房或制品广告",
		"CBD/汉麻制品（归 CBD & Hemp）；明显非大麻的普通植物叶子"},
	{"Adult Products",
		"展示或推广性玩具、情趣用品及其销售",
		"普通药品、医疗器械、保健品；普通内衣（若性暗示归 Sexually Suggestive）"},
	{"Adult Entertainment",
		"成人影像、色情站点、色情直播、付费陪聊等成人内容服务：色情站点标识/域名、成人内容预览、付费陪聊号召",
		"只销售成人用品（归 Adult Products）；非色情直播、视频平台或普通社交陪聊"},
	{"Profane Language",
		"文案或画面文字含脏话、侮辱性粗口或歧视性词语（包括 f*ck、sh*t 等遮罩写法）；竖中指等侮辱性手势",
		"无侮辱意图的轻度感叹或口头禅；有性暗示但没有脏话或侮辱手势"},
	{"Religion Spirituality",
		"宗教、信仰、灵修或占卜内容：经文引用、宗教符号配信仰内容、信仰号召、寺庙教堂传播劝募、付费占卜/塔罗/占星",
		"文化符号、宗教建筑偶然出现且无信仰内容；节日文化内容；无灵修内容的普通瑜伽/冥想健身"},
	{"Violence",
		"写实暴力或伤害：流血、伤口、殴打、枪击、刀刺、爆炸伤人、尸体、写实虐待描绘",
		"无写实流血伤害的游戏/动漫风格化战斗；单独枪械图标或展示且无暴力行为；普通体育对抗"},
	{"Scam/Fraud",
		"诈骗/欺诈或欺骗性内容：虚假中奖/领奖、仿冒官方机构或品牌索取钱财或个人信息、承诺保本高收益的虚假投资、刷单兼职骗局、假冒商品、名人虚假代言、免费试用订阅陷阱、虚假医疗功效宣称（治愈绝症/包治百病）、假系统弹窗/假关闭按钮/假播放键、假新闻式软文、耸动标题党",
		"正常促销折扣与限时优惠；持牌金融机构的常规理财广告；不误导事实的普通修辞夸张"},
	{"Political",
		"政治内容：候选人/政党竞选拉票、政治立场动员、政治捐款号召、堕胎/控枪/移民等政治议题倡导（含公益广告形式）、政治人物形象用于广告",
		"无政治内容的爱国装饰元素；非政治性公益公告（如交通安全、环保）"},
	{"CBD & Hemp",
		"展示或推广 CBD/汉麻制品：CBD 油/软糖/饮料、标注 CBD 或汉麻提取物的护肤与保健品、汉麻制品商店广告",
		"无 CBD 标注的普通汉麻纤维纺织品、汉麻籽食品；大麻/THC 制品（归 Marijuana）"},
	{"Medical Polarizing",
		"争议性医疗内容：反疫苗宣传、未经批准的疗法、干细胞/安乐死等伦理争议医疗议题的商业倡导",
		"普通保健品/健身/养生；正规医疗机构与药品的常规广告且无争议性宣称；堕胎议题倡导（归 Political）；治愈绝症/包治百病宣称（归 Scam/Fraud）"},
	{"Coronavirus",
		"新冠/疫情相关内容：宣称预防或治愈新冠的产品、疫情恐慌营销、疫情错误信息、疫情科普或新闻式素材",
		"口罩、消毒液等日用品的普通展示且无疫情关联文案；无疫情指向的医疗画面"},
	{"MILITARY VEHICLES",
		"展示或推广军用载具：坦克、装甲车、战斗机、军舰、导弹发射车等写实军事装备画面（含游戏/影视中的写实渲染）",
		"民用汽车/卡车/民航客机/民用船舶；明显的玩具模型或卡通风格化载具"},
}

// LabelCount 返回素材违规标签数量，供跨包测试动态断言。
func LabelCount() int { return len(visualLabels) }

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
			//加入素材违规标签问题 noul
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

// Submit 在有主体图片时调用 Hub，并按来源返回全部违规标签。
// 每张主体图片对每个标签都有一行结果：未送审或未取得有效分数的为 UNKNOWN，并带上 Reason。
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
				labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusUnknown, Reason: ReasonMissingAnswer, UpstreamModel: result.Model})
				continue
			}
			score := *answer.Noul
			labels = append(labels, LabelResult{Label: label.Name, SourceID: sourceID, Status: StatusScored, UpstreamModel: result.Model, Noul: &score})
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
