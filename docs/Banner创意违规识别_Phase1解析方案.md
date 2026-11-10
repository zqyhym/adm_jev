# Banner 创意违规识别 Phase 1 解析方案

> 范围：从 Bid Response 中提取 Banner 素材候选和来源。  
> 现有预览实现：`src/internal/creative/preview/preview_engine.go`。  
> 不下载图片，不调用 Hub，不改预览接口，不判断 11 类违规。

## 1. Phase 0 已确认的结论

内网联调已经跑通 `POST /aihub/api/v1/decision`。后面的解析按这些结果设计：

- 实际模型回显为 `typesafe/jev-1.13-20260917`。`noul` 是 0 到 1 的概率，文本工单可以按问题名解析。
- 不带 `TypeSafe-Danger-Zone-Images` 也会接受图片。同一道“是不是相机广告”的题，柯达广告 `0.51`，电台广告 `0.06`，去掉图片 `0.08`。画面参与了判断。`0.51` 没有达到通路测试里的 `0.7`，记为这张历史样本不确定，不记为通路失败。
- 宽 4183 的 `01_antique_shop_banner.jpg` 被 Hub 接受。尺寸限制留在本地 `ValidateImage`，不能依赖 Hub 拒收超限图。
- 11 类标签的正反例，以及 `nurl`、`burl` 是否计入 Unsecured Request，仍等待业务确认。这两项不挡住素材解析。

## 2. 目的

给定一条 Bid Response，或单独一段 Banner ADM，列出每个 bid 里可供后续送审的素材候选。每个候选都能回到字段路径和 ADM 中的位置。解析失败保持失败状态，不能变成“无违规”。

本阶段只产生来源清单。图片是否真的下载成功、内容 hash、送审和 URL 协议判断都放到后续阶段。

## 3. 不做什么

- 不覆盖 Video、Native 的审核提取。预览里已有的 VAST、Native 解析保持原样，不纳入本阶段验收。
- 不执行第三方 JavaScript，不渲染 HTML5，不处理 MRAID 环境。脚本和 MRAID 只标记依赖。
- 不下载图片或 Iframe 页面，因此本阶段没有素材内容 hash。
- 不调用 JEV，不输出 `HIT`、`NOT_HIT` 或 `UNKNOWN` 标签。
- 不修改 `/api/preview` 的返回。预览继续可以改写 tracker、重写 `mraid.js`，并在缺尺寸时使用 300×250。

## 4. 和预览解析分开

`preview.Parse` 已经能识别 Banner 图片、可见文案、tracker 和 Jampp `core_main_image`。审核提取复用这些判断，但不直接使用 `model.Result`：

- 预览只读取 `seatbid[0].bid[0]`。审核要遍历每一个 bid。
- 预览在 `w`、`h` 都缺失时填入 300×250。审核保持 0，表示响应和标签都没有给出尺寸。
- 预览会改写 ADM 并生成预览页。审核保留原始 ADM，只计算 ADM 的 hash。
- 预览跳过 `script` 和 `style` 是对的，可见文案不能从这里来。审核另外把脚本地址、Iframe 地址和声明的资源 URL 记成候选或动态依赖。
- HTML 解析失败时，预览仍可能返回一条 banner。审核要把这种输入标成 `FAILED` 或 `PARTIAL`。

tracker 判断沿用预览里已经覆盖的规则：地址包含 `impression.php`、`burl.php`、`click.php`、`simp.png`、`mmp_imp`，样式把元素放到 `left:-9999px`，或宽高都是 0 或 1。Jampp `core_main_image` 继续还原 `imgsrv` 后面的 CDN 原图，并标成图片候选。它来自动态 token，`obtained` 仍为 false。

## 5. 输入

两种输入都要接受：

- OpenRTB Bid Response JSON。读取全部 `seatbid[].bid[]` 的 `adm`、`crid`、`adomain`、`w`、`h`、`impid`。`nurl`、`burl` 只记录地址和字段路径，不在本阶段判断协议。
- 裸 ADM 字符串。没有 Response 字段时，对应元数据留空。

可选的第二段输入是原始 Bid Request。只有 Request 和 bid 的 `impid` 对得上时，才写入该 `imp` 的 `secure`。对不上或没有 Request 时，`impSecure` 为空，不补造。

## 6. 输出

每个 bid 一条记录：

| 字段 | 含义 |
| --- | --- |
| `bidPath` | 例如 `seatbid[0].bid[1]`；裸 ADM 为 `adm` |
| `crid` | 创意标识，可空 |
| `adomain` | 取 `adomain` 的第一项，可空 |
| `w` / `h` | 响应或图片标签中的尺寸；没有则为 0 |
| `impid` | bid 上的展示位标识，可空 |
| `impSecure` | 仅在关联到 Request 时为 true 或 false，否则为空 |
| `admHash` | 原始 ADM 的 SHA-256 |
| `admTypes` | `HTML`、`IMAGE`、`SCRIPT`、`IFRAME`、`MRAID` 中实际出现的项，可同时存在 |
| `parseStatus` | `OK`、`PARTIAL` 或 `FAILED` |
| `failure` | 失败原因；成功时为空 |
| `assets` | 来源列表 |

每条来源：

| 字段 | 含义 |
| --- | --- |
| `sourceId` | 同一次提取内稳定，建议 `bidPath` 加序号 |
| `kind` | `image`、`text`、`script`、`iframe`、`url` |
| `value` | 图片或资源地址，或可见文案 |
| `fieldPath` | 例如 `seatbid[0].bid[0].adm` |
| `role` | `creative`、`tracker` 或 `dynamic` |
| `obtained` | 本阶段图片和远程页面都还没下载，一律为 false |

`admHash` 只代表原始 ADM 文本。图片内容 hash 等字节真正落到本地后再生成。

### 状态

- `OK`：ADM 可以解析，可见素材或明确的主体图片候选已经列出。
- `PARTIAL`：结构可以识别，但主体素材还依赖脚本、Iframe 或未执行的 token，当前清单不完整。
- `FAILED`：ADM 为空，或 HTML 无法形成可审核结构。

这三个状态都不是合规结论。

## 7. 提取规则

1. 可见文案只来自 HTML 文本节点。`script`、`style` 和注释里的文字不进入 `text`。
2. `<img>` 的 `src` 和 `srcset`，以及样式里明确写出的图片地址，记为 `image`。tracker 的 `role` 为 `tracker`，其余为 `creative`。
3. `<script src>` 记为 `script`，`role` 为 `dynamic`。内联脚本不把脚本正文当成文案；能静态识别出的图片地址可以另列为 `image`，例如 Jampp 主图。
4. `<iframe src>` 记为 `iframe`，`role` 为 `dynamic`。不抓取框架内的页面。
5. `<a href>` 以及 `nurl`、`burl` 记为 `url`，并保留字段路径。协议是否违规不在本阶段判断。
6. 出现 `mraid.js` 或 `mraid.` 时，在 `admTypes` 中加入 `MRAID`。不把它展开成已取得素材。
7. 同一个 bid 内相同地址只保留一条。若任一处被判定为 tracker，结果保留 tracker。
8. 没有任何可审核主体，只有 tracker、脚本或 Iframe 时，状态为 `PARTIAL`，失败原因说明缺少已取得的主体素材。

## 8. 验收用例

基线来自 `src/internal/creative/preview/preview_engine_test.go`。实现时预览测试的现有预期保持不变。

| 编号 | 输入 | 预期 |
| --- | --- | --- |
| P1-01 | 静态图：`<img src="https://cdn.example/a.jpg" width="320" height="50">`，链接落地页，可见文案“秋季促销” | `OK`。图片来源一条，文案一条，落地页一条 `url`。尺寸 320×50。图片 `obtained=false` |
| P1-02 | 两个 tracker：移出屏幕的 `impression.php`，以及 1×1 的 `mmp_imp` | 两条图片都是 `tracker`。没有主体素材，状态为 `PARTIAL` |
| P1-03 | Jampp Response：`crid=6868058`，`adomain=autodoc.de`，300×250，含隐藏 impression、`mraid.js` 和 `core_main_image` | 保留 crid、adomain 和尺寸。主图还原为 `https://cdn.jampp.com/assets/1626/Purple221-1781688235340.jpg`，`role=creative`。impression 为 tracker。脚本和 MRAID 为 dynamic。状态为 `PARTIAL` |
| P1-04 | 一个 Response 里两个 bid，ADM 各含一张不同图片 | 两条记录，`bidPath` 分别为 `seatbid[0].bid[0]` 和 `seatbid[0].bid[1]`，图片地址不串 |
| P1-05 | ADM 只有 `<iframe src="https://ads.example/frame.html">` | 记录 Iframe URL，`role=dynamic`。不把框架内容当成已审核素材。状态为 `PARTIAL` |
| P1-06 | ADM 为空，或 Response 中某个 bid 的 `adm` 为空 | 该 bid 为 `FAILED`，来源为空。不生成无违规结论 |
| P1-07 | 图片标签没有宽高，Response 也没有 `w`、`h` | `w` 和 `h` 为 0，不填 300×250 |
| P1-08 | bid 带 `impid`，Request 中对应 `imp.secure=1` | `impSecure=true`。没有 Request 的同一条 Response，`impSecure` 为空 |
| P1-09 | Response 带 `nurl=http://notice.example/win` | 记录为 `url`，字段路径指向 `nurl`。不判断 HTTP 是否违规 |

## 9. 退出判定

同时满足以下条件，Phase 1 完成：

- P1-01 到 P1-09 的来源都能追溯到 bid 和字段路径。
- 静态图片和简单 HTML 的重复解析结果一致。
- 空 ADM、只有 tracker、只有 Iframe 或脚本依赖的样本不会被记成无违规。
- `/api/preview` 的现有测试仍然通过。

动态 ADM 的隔离渲染、图片下载、内容 hash、Unsecured Request 规则和 JEV 送审不属于本阶段。
