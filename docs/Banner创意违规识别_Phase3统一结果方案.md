# Banner 创意违规识别 Phase 3 统一结果方案

> 范围：把已取得的创意图片和可见文案收成 11 类结果。  
> 送审入口：`src/internal/creative/asset/review.go`。  
> URL 分组：`src/internal/creative/asset/scheme.go`。  
> 不改现有预览接口，不渲染 MRAID，不调用本文件编写时的 Hub。真实图片的 11 类联调留到方案确认后的实现阶段。

## 1. 当前缺口

`Reviews` 现在只问一个问题：“画面是否可辨认。” Phase 2 收口时，柯达样图返回 `noul=0.39`，Liftoff 预览图返回 `noul=0.34`。这两次只说明图片能按 `sourceId` 送到 `jev-1.13`，不是违规标签。

Phase 3 用 10 个视觉/语义 `noul` 替换这个问题，并与已经分开的 URL 规则合成 11 类结果。

## 2. 标签来源

命中定义和排除条件沿用《Banner 创意违规识别（JEV 决策模型）递进式建设计划》第 4 节。开发样本来自公有领域或 CC0，只用于对照请求是否按标签返回分数。它们不代替业务对正反例的确认，本方案也不把 `0.85`、`0.30` 写成自动 `HIT`、`NOT_HIT` 的定稿阈值。

10 个交给模型的标签：

| 标签 | 命中要点 | 排除要点 |
| --- | --- | --- |
| Dating | 约会、相亲、征友、婚恋、匹配服务 | 情侣或婚礼内容本身不是约会服务 |
| Gambling | 赌场、下注、彩票、竞猜、赢钱号召 | 普通游戏和无赢钱承诺的虚拟金币 |
| Nudity | 可见裸体，或裸露胸部、臀部、生殖器 | 普通暴露服装、泳装本身 |
| Sexually Suggestive | 未裸体，但姿势、服装、镜头或文案有明显性暗示 | 普通泳装、健身且无性暗示 |
| Marijuana | 明确推广大麻、THC、相关药房、吸食或制品 | 只有叶子图案、没有相关文案或符号 |
| Adult Products | 性玩具、情趣用品 | 普通药品 |
| Adult Entertainment | 成人影像、色情站点、色情直播或陪聊 | 只销售成人用品、没有成人内容或陪聊 |
| Profane Language | 文案或画面文字含脏话、侮辱性粗口 | 有性暗示但没有脏话 |
| Religion Spirituality | 宗教、信仰、灵修、占卜、塔罗，或寺庙、教堂相关传播和劝募 | 文化符号或建筑偶然出现、没有信仰号召 |
| Violence | 流血、伤口、殴打、枪击、刀刺、爆炸伤人或尸体 | 单独的枪械图标、普通体育对抗 |

Unsecured Request 不交给模型。

## 3. 每张图片的模型请求

只对 `kind=image`、`role=creative`、`obtained=true` 的图片发请求。一张图片一次请求，内含 10 个 `noul`。`state` 保留现有的图片引用、`sourceId` 和可见文案。

每个问题的 `instructions` 写明标签名。`criteria.true` 使用上表的命中要点，`criteria.false` 使用排除要点。题干要求只根据该图片和同条请求中的文案判断。

没有已取得的主体图片时不调用 Hub。动态依赖未展开时，已取得的 `iurl` 或静态主图仍可单独送审，但 bid 的解析状态保持 `PARTIAL`，不能改成素材已完整。

## 4. 统一结果

每张已取得图片的每个标签保存：

| 字段 | 含义 |
| --- | --- |
| `label` | 上表中的标签名 |
| `sourceId` | 送审图片来源 |
| `status` | `SCORED` 或 `UNKNOWN` |
| `noul` | 模型返回且位于 0 到 1 的分数；否则为空 |
| `probability` | 本阶段固定为空 |

`SCORED` 只表示分数可解析并已挂上 `sourceId`。它不是 `HIT`，也不是 `NOT_HIT`。未校准的 `noul` 不叫概率。

以下情况该标签为 `UNKNOWN`，`noul` 为空：

- 模型没有返回这个标签。
- 返回的分数缺失或不在 0 到 1。
- 响应无法对应到本次送审的 `sourceId`。
- 图片未取得、下载失败，或 bid 没有可送审主体图。

这些情况不能补成 `NOT_HIT`。

同一 bid 有多张已取得图片时，按 `sourceId` 分别保留 10 类结果，不合并成一个更宽松的未命中。

## 5. Unsecured Request

这一类来自 `Classify` 的创意资源分组，不来自模型。

- 创意资源中有 `http://`：`HIT`，证据带上对应 `sourceId`、地址和 scheme。
- 创意地址都已解析，且没有 `http://`：`NOT_HIT`。
- 创意地址无法解析，或清单不完整：`UNKNOWN`。
- `nurl`、`burl`、`lurl` 继续放在通知地址统计里。即使它们是 `http://`，也不并入本标签。
- `imp.secure` 缺失本身不构成命中。Hub 的内网地址不参与这个标签。

地址样本：

- 创意图片 `http://cdn.example/a.jpg`：Unsecured Request 为 `HIT`。
- 同一条里的 `nurl=https://notice.example/win`：通知地址，不命中。
- 来源清单里没有的 `http://`，即使出现在注释或脚本正文中：不产生候选。

## 6. 开发样本

目录：`label-samples/`。文件只进入开发集，不进入最终测试集。来源、许可和标签角色写在该目录的 `sources.txt`。

已下载：

- `gambling_lottery_ad_1902.jpg`：1902 年彩票广告，Gambling 正例。
- `dating_lets_get_married_1926.jpg`：1926 年婚姻喜剧广告，Dating 正例，接近“婚礼本身不是约会服务”的边界。
- `religion_billy_sunday_preaching.jpg`：福音布道家讲道形象，Religion Spirituality 正例。
- `violence_harvest_of_death_1863.jpg`：1863 年战场死亡照片，覆盖 Violence 的尸体画面。它不是广告。
- `marijuana_hasheesh_ad_1880.png`、`marijuana_cannabis_cigarette_ad_1881.jpg`：Marijuana 正例。`marijuana_medicine_bottle.jpg` 是药品瓶边界。
- `suggestive_moulin_rouge_poster_1891.jpg`：Sexually Suggestive 和 Adult Entertainment 正例。海报中的舞者着装，不作为 Nudity 正例。
- `profane_language_1785.txt`：1785 年公有领域粗口词条。配对的普通画面使用 `ad-creative-samples/05_kodak_print_ad_1920.jpg`。

`ad-creative-samples/` 里的柯达、电台、古董店和广告牌继续作为普通商业广告反例，不重复下载。

未找到合适的公有领域文件，因此没有下载：Nudity 正例、Adult Products 正例，以及 Dating、Gambling、Religion、Violence 的专门边界图。Unsecured Request 不下载图片，使用第 5 节的地址样本。

## 7. 不做什么

- 不渲染 MRAID，不执行脚本，不解码脚本里的内嵌画面。
- 不做 OCR，不接 TMT。
- 不把开发样本上的分数写成准确率，也不用统一阈值自动拦截。

## 8. 验收

实现阶段用本地假 Hub 验证：

| 编号 | 输入 | 预期 |
| --- | --- | --- |
| P3-01 | 一张已取得的创意图 | 请求含 10 个 `noul`，题干包含对应标签的命中和排除要点，`state` 含 `sourceId` |
| P3-02 | 假 Hub 少返回一个标签 | 该类为 `UNKNOWN`，其余可解析标签为 `SCORED`，不出现 `NOT_HIT` |
| P3-03 | 没有 `obtained=true` 的主体图片 | 不请求 Hub，10 类视觉标签为 `UNKNOWN` |
| P3-04 | 创意图为 `http://cdn.example/a.jpg`，`nurl` 为 `https://notice.example/win` | 只有创意图使 Unsecured Request 为 `HIT` |
| P3-05 | 创意地址都是 `https://`，通知地址另有 `http://` | Unsecured Request 为 `NOT_HIT`，通知地址不并入 |
| P3-06 | 同一 bid 两张已取得图片 | 两类结果按各自 `sourceId` 保留 |

`/api/preview` 和 Phase 2 的现有测试保持通过。真实样本的 11 类联调在上述本地验收通过后再做。
