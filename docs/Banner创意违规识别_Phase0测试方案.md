# Banner 创意违规识别 Phase 0 测试方案

> 范围：Hub 决策接口的文本、单图和图文对照。  
> 对应代码：`src/internal/jev`。  
> 不覆盖 ADM 解析、11 类准确率、TMT 对照和线上拦截。  
> `POST /v1/responses` 是另一套接口，本阶段不测。

## 1. 目的

证明 `jev-1.13` 经自有 Hub 的决策接口能接收文本和真实图片，并返回可按问题名解析的 `noul`、`choice`、`score`。图像必须实际影响判断。调用失败、图片被拒和缺项都不能变成 `NOT_HIT`。

一次成功只说明通路可用，不说明审核准确，也不说明支持区域框或自由文本解释。

## 2. 前置条件

- Hub 地址走内网。本阶段直接使用 `http://49.0.206.228:80`，不再单独确认 HTTPS。
- 密钥只放在运行环境的 `AIHUB_API_KEY`。不写入仓库、日志、错误信息和前端。
- 可选覆盖：`AIHUB_BASE_URL`、`AIHUB_MODEL`。未设置时分别为 `http://49.0.206.228:80` 和 `jev-1.13`。

## 3. 接入

| 项 | 值 |
| --- | --- |
| 方法 | `POST` |
| 路径 | `/aihub/api/v1/decision` |
| 完整地址 | `http://49.0.206.228:80/aihub/api/v1/decision` |
| 鉴权 | `Authorization: Bearer $AIHUB_API_KEY` |
| 内容类型 | `application/json` |
| 模型 | `jev-1.13` |
| 图片头 | `TypeSafe-Danger-Zone-Images: true`，是否必须由用例 P0-06 记录 |

请求体包含 `model`、`state`、`questions`。图片放入 `assets`，值为 `type=image`、`format=jpg`、`data_base64`。`state` 中用独立引用指向该图片，引用不出现在题干文字里。

`noul` 表示“为真”的概率，只有落在 0 到 1 时才记为命中概率。`choice` 是选项名，`score` 是等级期望值。Hub 若未回这些字段，保持空值，不从解释文字补造。

## 4. 样本

目录：`ad-creative-samples/`。这些图只进开发集。

| 文件 | 尺寸 | 用途 |
| --- | --- | --- |
| `02_kodak_print_ad_1921.jpg` | 935×1421 | 图文组合 |
| `04_wjr_wgar_radio_ad_1939.jpg` | 685×948 | 换图对照 |
| `05_kodak_print_ad_1920.jpg` | 580×822 | 单图、去图对照、图片头 |
| `01_antique_shop_banner.jpg` | 4183×603 | 超限拒绝 |
| `03_swift_billboard_circa_1926.jpg` | 3000×2110 | 超限备用，自动化用例使用 `01` |

上游限制：JPEG 或 PNG，宽和高都在 280 到 1568 像素之间，总像素 78400 到 2458624。超限整单拒绝，不会自动缩放。本地 `ValidateImage` 按 JPEG 和该尺寸范围提前拒绝；联调仍把 `01` 原图发给 Hub，确认服务端拒绝。

## 5. 结果判读

- 已解析的问题状态为 `ANSWERED`。未返回的问题状态为 `UNKNOWN`。
- 本阶段客户端不输出 `HIT` 或 `NOT_HIT`。
- 非 2xx、图片被拒、JSON 无法解析，返回错误，结果不能当成未命中。
- 单图通路的自动判定只用于证明画面参与判断：相机广告 `noul >= 0.7`，电台广告 `noul <= 0.3`，且前者高于后者。这不是 11 类准确率门槛。

## 6. 用例

失败时记录 HTTP 状态码、实际 `model` 字段和脱敏后的响应正文。正文中的密钥替换为 `[REDACTED]`。

### P0-01 文本冒烟

- 输入：Hub 工单示例。`state.customer_tier=enterprise`，`state.ticket` 为结账页点击 Pay 后白屏。问题为 `is_bug`（noul）、`team`（choice：account / frontend / payments）、`urgency`（score：next release / this week / blocking revenue）。
- 操作：不带图片调用决策接口。
- 预期：三个问题都能按名称解析；`noul` 在 0 到 1；`choice` 非空；`score` 为数值。记录实际模型字段。
- 代码：`TestLivePhase0/text`。

### P0-02 单图

- 输入：`05_kodak_print_ad_1920.jpg`。问题 `camera_ad`：“这是不是相机广告？” `criteria.true` 为相机或摄影产品广告，`criteria.false` 为不是相机广告。请求带头 `TypeSafe-Danger-Zone-Images: true`。
- 预期：`camera_ad` 为 `ANSWERED`，`noul >= 0.7`。
- 代码：`TestLivePhase0/single_image`。

### P0-03 换图对照

- 输入：同一问题和同一请求头，图片改为 `04_wjr_wgar_radio_ad_1939.jpg`。
- 预期：`noul <= 0.3`，并且低于 P0-02。两次结果随画面变化。
- 代码：`TestLivePhase0/swap_image`。

### P0-04 去图对照

- 输入：同一问题，`state.creative` 为“未提供图片”，不传 `assets`。
- 预期：请求失败，或返回的 `noul` 低于 P0-02。去掉图片后不再给出与有图相机广告相同的高概率。
- 代码：`TestLivePhase0/remove_image`。

### P0-05 图文

- 输入：`02_kodak_print_ad_1921.jpg`，可见文案 `Eastman Kodak`。问题为图片和文案是否都在推广相机。带头。
- 预期：问题可解析，`noul` 在 0 到 1。
- 代码：`TestLivePhase0/image_and_text`。

### P0-06 图片请求头

- 输入：与 P0-02 相同的图片和问题，不带 `TypeSafe-Danger-Zone-Images`。
- 预期：记录被接受或被拒绝的状态码。P0-02 已经覆盖带头请求。连接失败视为用例失败；Hub 返回 4xx 或成功解析都记入结果。
- 代码：`TestLivePhase0/header_off`。

### P0-07 坏图

- 输入：截断 JPEG、GIF、宽或高不在 280 到 1568 的 JPEG、样本 `01`。
- 预期：本地 `ValidateImage` 和 `Decide` 直接返回错误，不发起 HTTP 请求。联调时 `01` 仍提交给 Hub，预期非 2xx。两类失败都不产生 `NOT_HIT`。
- 代码：`TestValidateImage`、`TestDecideRejectsBadImageLocally`、`TestLivePhase0/oversize_image`。

### P0-08 鉴权与路由

- 输入：本地假服务检查路径、模型、鉴权头和图片 base64。联调使用密钥 `invalid-phase0-key` 发送一条文本请求。
- 预期：路径为 `/aihub/api/v1/decision`，模型为 `jev-1.13`，鉴权头为 Bearer。错误密钥得到非 2xx。错误文本不包含真实密钥。
- 代码：`TestDecideRequestShape`、`TestErrorRedactsAPIKey`、`TestLivePhase0/bad_key`。

### P0-09 缺项响应

- 输入：请求两个问题，假服务只返回一个。
- 预期：已返回的问题为 `ANSWERED`。未返回的问题为 `UNKNOWN`，同时返回缺少结果的错误。不补成未命中。
- 代码：`TestMissingAnswerStaysUnknown`。

## 7. 本地用例与联调的分工

| 测试 | 网络 |
| --- | --- |
| `TestNewFromEnvDefaults` | 否 |
| `TestDecideRequestShape` | 否，`httptest` |
| `TestValidateImage` | 否 |
| `TestDecideRejectsBadImageLocally` | 否 |
| `TestMissingAnswerStaysUnknown` | 否 |
| `TestErrorRedactsAPIKey` | 否 |
| `TestLivePhase0` | 是。未设置 `AIHUB_API_KEY` 时跳过 |

## 8. 执行

在 `src` 目录执行：

```text
go test ./internal/jev/
```

未设置密钥时，输出包含“跳过内网联调”，本地用例仍须通过。设置密钥后，同一命令会访问内网 Hub。

## 9. 退出判定

同时满足以下条件，Phase 0 通路验证通过：

- P0-02 与 P0-03 的 `noul` 随相机广告和电台广告反向变化。
- 文本、单图和图文结果能按问题名解析。
- P0-04 去掉图片后不再维持有图时的高概率。
- P0-07、P0-08、P0-09 的失败和缺项保持错误或 `UNKNOWN`。
- 日志和错误信息中没有密钥。

仍以当次响应为准、不在本方案中预设的字段：Hub 是否改写 `answers` 的外层结构、实际回显的模型版本、不带图片头时是否接受图片。这些由联调日志记录，不把未出现的分数、解释或区域框补进结果。
