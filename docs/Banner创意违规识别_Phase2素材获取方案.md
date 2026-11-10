# Banner 创意违规识别 Phase 2 素材获取方案

> 范围：补齐可送审的静态图片来源，安全下载并计算内容 hash，按来源组装决策请求，并把创意 URL 与通知 URL 分开做协议判断。  
> 上游提取：`src/internal/creative/preview/extract.go`。  
> 不渲染 MRAID，不执行 ADM 脚本，不改 `/api/preview`。

## 1. 为什么现在做

Phase 1 已经能列出静态图片、可见文案、脚本、Iframe 和 `nurl`、`burl`。昨天的 Liftoff Banner（`crid=471781`，`king.com`，320×480）被标为 `PARTIAL`：主体画面在脚本和 MRAID 里，清单不完整。

这条响应同时带有预览图：

`iurl = https://cdn.liftoff-creatives.io/customers/417/previews/15c1d3a623-ios-f8f3e8804a.png`

当前提取没有读取 `iurl` 和 `lurl`。送审前必须把这张预览图纳入来源，否则模型只能看到追踪像素和脚本地址。

## 2. 目的

对 Phase 1 的来源清单做三件事：

1. 补上 `iurl` 和 `lurl`。
2. 下载可送审的图片，成功后写入内容 hash，并把对应来源的 `obtained` 设为 true。
3. 只把已取得的图片和可见文案交给已验证的 Hub 决策接口。创意资源里的 `http://` 单独记为 Unsecured Request 候选。

没有取得主体图片时不调用 Hub。下载失败、图片不可读或仍有动态依赖，都保持不完整，不能变成未违规。

## 3. 不做什么

- 不执行 ADM 里的 JavaScript，不渲染 MRAID，不解码脚本中的内嵌 HTML。Candy Crush 画面仍留到动态素材专项。
- 不下载 `role=tracker` 的图片，避免请求 `impression.php` 等地址触发真实曝光。
- 不下载脚本、Iframe 和点击地址。
- 不把 `nurl`、`burl`、`lurl` 并入 Unsecured Request。业务尚未确认通知地址是否算违规。
- 不输出 11 类标签。本阶段送审只验证图文是否按 `sourceId` 送入，沿用 Phase 0 的单问题通路。
- 不做 OCR。
- 不修改预览接口的返回、默认尺寸和 tracker 改写。

## 4. 补齐 iurl 和 lurl

在 `extract.go` 读取 bid 的 `iurl`、`lurl`，规则与现有 `nurl`、`burl` 相同：只记录响应字段，不发请求。

| 字段 | kind | role | 字段路径 | 是否下载 |
| --- | --- | --- | --- | --- |
| `iurl` | `image` | `creative` | `seatbid[0].bid[0].iurl` | 是 |
| `lurl` | `url` | `creative` | `seatbid[0].bid[0].lurl` | 否，归入通知地址 |

`iurl` 写入时 `obtained=false`。它和 ADM 里的 `<img>` 一样参与去重：同一地址只留一条，任一处被判定为 tracker 则保留 tracker。

Liftoff 样本因此至少新增一条图片来源，值为上述 png 地址。状态仍可为 `PARTIAL`，因为脚本和 MRAID 依赖还在。

## 5. 安全下载

只处理 `kind=image` 且 `role=creative` 的来源。

下载约束：

- 只接受 `http` 和 `https`。相对路径、空地址记为未取得。
- 每次请求限制跳转次数。每一跳都重新检查目标主机，禁止访问本机、链路本地、私网和云元数据地址。
- 响应体设置上限。超过上限或读不完整，视为失败。
- 内容必须是 JPEG 或 PNG。类型以文件内容为准，不以 URL 后缀为准。
- 送入 Hub 前仍用 `src/internal/jev/client.go` 的尺寸规则：宽和高都在 280 到 1568 像素之间。超限不送给模型，并在来源上留下失败原因。Hub 曾经接受宽 4183 的图，不能依赖服务端拒收。

成功时：

- 计算图片字节的 SHA-256，写入该来源的内容 hash。这与 ADM 文本 hash 分开。
- `obtained=true`。
- 保存的字节就是之后送审的字节。

失败时：

- `obtained` 保持 false。
- 记录失败原因，例如超时、非图片、超限、目标地址被拒绝。
- 不把该来源补成未命中。

## 6. 送审输入

调用 Phase 0 已验证的 `POST /aihub/api/v1/decision`，模型仍是 `jev-1.13`。图片放在 `assets`，`state` 里用不会和正文冲突的引用指向它。

组装规则：

- 一个 `sourceId` 对应一份图片字节。引用字符串必须出现在该来源的 `state` 中。
- 可见文案来自 `kind=text` 的来源，原样放进 `state`。没有文案时只送图片。
- 没有 `obtained=true` 的主体图片时，不调用 Hub。
- 多张主体图片分开请求，或在同一次请求里使用不同引用。返回结果必须能回到 `sourceId`。对不上时，该图片的判断记为未知。
- 动态依赖仍在时，可以送已取得的 `iurl` 或静态主图做通路验证，但整个 bid 不能因此改成“素材已完整”。

## 7. URL 协议分组

只检查来源清单里的地址，不对 ADM 原文做 `http://` 字符串搜索。

| 分组 | 来源 | 本阶段结论 |
| --- | --- | --- |
| 创意资源 | `role=creative` 的图片、脚本、Iframe、点击 URL，含 `iurl` | `http://` 记为 Unsecured Request 候选；`https://` 不命中 |
| 通知地址 | 字段路径为 `nurl`、`burl`、`lurl` 的 URL | 单独统计。即使是 `http://`，也不并入 Unsecured Request |
| 无法解析 | 相对路径、空 scheme、明显的动态占位且不能还原成绝对 URL | 记为待确认，不记为合规 |

`imp.secure` 缺失本身不构成命中。Hub 的内网地址不属于广告素材，不参与这个标签。

## 8. 验收用例

| 编号 | 输入 | 预期 |
| --- | --- | --- |
| P2-01 | bid 含 `iurl=https://cdn.example/preview.png` | 新增图片来源，`role=creative`，`obtained=false`，字段路径以 `.iurl` 结尾 |
| P2-02 | 同一 bid 含 `lurl=https://rtbnotify.example/loss` | 记为 `url`，字段路径以 `.lurl` 结尾，不进入下载列表 |
| P2-03 | 本地测试服务返回一张 320×320 JPEG | 对应来源 `obtained=true`，内容 hash 等于响应字节的 SHA-256 |
| P2-04 | 图片地址返回 404、截断内容或 HTML | `obtained` 仍为 false，并有失败原因 |
| P2-05 | 图片地址指向 `127.0.0.1`、`10.0.0.1` 或云元数据地址 | 不发起对目标内容的使用，来源保持未取得 |
| P2-06 | 创意图片 URL 为 `http://cdn.example/a.jpg`，`nurl` 为 `https://notice.example/win` | 只有创意图进入 Unsecured Request 候选；`nurl` 留在通知地址统计 |
| P2-07 | ADM 注释或脚本文本里出现 `http://`，来源清单没有这个地址 | 不产生 Unsecured Request 候选 |
| P2-08 | Liftoff 样本的字段形态：有 `iurl`、隐藏 `impression.php`、`mraid.js` | `iurl` 被列出；`impression.php` 仍是 tracker 且不下载；解析状态仍可为 `PARTIAL` |
| P2-09 | 没有任何 `obtained=true` 的主体图片 | 不调用 Hub |

`/api/preview` 的现有测试保持通过。

## 9. 退出判定

同时满足以下条件，Phase 2 完成：

- `iurl` 可追溯，`lurl` 不会被当成送审图片。
- 下载成功与失败可区分，成功样本的 hash 能对上实际字节。
- 创意资源的 `http://` 与通知地址不会混成同一个标签。
- 缺图和动态依赖不会被记成未命中或未违规。
- 已取得的图片能按 `sourceId` 放进决策请求；没有主体图片时不会调用 Hub。
