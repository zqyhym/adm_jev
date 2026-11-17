# adm_jev

广告素材（ADM）预览，并用 JEV 快速判断 ADM 是否合规。

输入可以是 OpenRTB Bid Response，也可以是裸 ADM。页面分三块：素材预览、素材审核、审核报表。审核链路解析素材、下载 Banner 创意图片，把 10 类视觉/语义标签交给自有 Hub 上的 JEV，再用规则判断创意资源是否使用了不安全的 `http://`。

当前范围是 Banner 静态素材的离线验证，不改线上审核决策。

## 功能

| 页面 | 地址 | 做什么 |
| --- | --- | --- |
| 素材预览 | `#/preview` | 解析 ADM，还原画面、图片、文案和被拦截的资源 |
| 素材审核 | `#/review` | 下载创意图片，返回 10 类标签分数和 Unsecured Request 结论，并写入审核记录 |
| 审核报表 | `#/report` | 按时间、状态、关键词查看已落库的审核记录 |

预览只取第一席位的第一条 bid。审核会处理响应里的全部 bid。

## 目录

```text
src/cmd/server          HTTP 服务，默认 :8080
src/cmd/eval            静态图片离线评测
src/internal/creative   预览、审核编排、标签与 URL 规则
src/internal/material   Bid Response / ADM 解析和图片下载
src/internal/jev        Hub 决策客户端
src/internal/platform   配置与 MongoDB
web/                    Vue 3 + Vite + Element Plus
docs/                   Banner 违规识别建设方案
datasets/               离线评测样本（不入库）
eval-runs/              评测输出（不入库）
```

## 环境

- Go 1.23
- Node.js（前端）
- MongoDB，启动时必须能连上
- 审核和离线评测需要 Hub 密钥 `AIHUB_API_KEY`

## 启动

开发时开两个终端。前端把 `/api` 和 `/mraid.js` 代理到 `127.0.0.1:8080`。

```powershell
cd web
npm install
npm run dev
```

```powershell
cd src
$env:AIHUB_API_KEY = "<密钥>"
go run ./cmd/server -dev
```

浏览器打开 `http://127.0.0.1:5173`。`-dev` 把页面请求反代到 Vite（默认 `http://127.0.0.1:5173`）。

不用 Vite 时，先构建再启动，页面由 Go 直接提供 `web/dist`：

```powershell
cd web
npm run build
cd ../src
go run ./cmd/server
```

常用参数：

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-addr` | `:8080` | 监听地址 |
| `-dev` | 关闭 | 页面反代到 Vite |
| `-vite` | `http://127.0.0.1:5173` | Vite 地址 |

## 环境变量

| 变量 | 默认 | 用途 |
| --- | --- | --- |
| `ADM_JEV_MONGO_URI` | `mongodb://127.0.0.1:27017` | Mongo 地址 |
| `ADM_JEV_MONGO_DATABASE` | `adm_jev` | 库名，审核记录在集合 `adm_review_records` |
| `AIHUB_BASE_URL` | 内网 Hub | 决策接口根地址 |
| `AIHUB_MODEL` | `jev-1.13` | 请求模型 |
| `AIHUB_API_KEY` | 空 | Bearer 密钥。审核可以启动，但没有密钥时模型调用会失败 |

密钥只放环境变量，不写入库和日志。

## 接口

请求体都是 JSON：`{"input":"<Bid Response 或裸 ADM>"}`，上限 8MB。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/preview` | 预览结果：格式、尺寸、图片、文案、拦截项、`previewHtml` |
| `POST` | `/api/review` | 审核结果。输入为空或没有可审核 bid 时返回 400，并尽量落一条失败记录 |
| `GET` | `/api/review/records` | 报表。查询参数：`from`、`to`（`YYYY-MM-DD`，须同时给）、`status`（`success` / `warning` / `failed`）、`q`、`page`、`pageSize`（1–100）。默认最近 7 天，跨度不超过 90 天 |
| `GET` | `/api/review/records/:id` | 单条记录，含原文和完整结果 |
| `ANY` | `/api/empty` | 1×1 透明 GIF，预览里替换追踪像素 |
| `ANY` | `/mraid.js` | 预览用的 MRAID 桩 |

审核成功时，10 类视觉标签的 `status` 为 `SCORED`（带 `noul`）或 `UNKNOWN`（带 `reason`）。`UNKNOWN` 表示没送审或没拿到分数，不表示未命中。`probability` 本阶段为空。

Unsecured Request 只看创意资源 URL 的协议，`nurl` / `burl` / `lurl` 单独放在 `notices`，不并入该标签。结论为 `HIT`、`NOT_HIT` 或 `UNKNOWN`。

记录状态：

| status | 含义 |
| --- | --- |
| `success` | 解析并完成审核 |
| `warning` | 已返回部分结果，但下载或模型有 `reviewError` |
| `failed` | 输入无效或没有可审核 bid |

送审图片须为 JPEG 或 PNG，宽高和边长都在 256 到 1568 像素之间。超出范围、损坏或未下载到的图片记为 `UNKNOWN`，不发给 Hub。

## 标签

JEV 判断这 10 类：Dating、Gambling、Nudity、Sexually Suggestive、Marijuana、Adult Products、Adult Entertainment、Profane Language、Religion Spirituality、Violence。

第 11 类 Unsecured Request 由 URL 规则生成，不经过模型。

## 离线评测

样本放在 `datasets/static-v1/images/<label>_pos` 和 `<label>_neg`。目录名只约束对应标签的真值，其余标签视为未标注。

在 `src` 目录执行，路径相对仓库根：

```powershell
$env:AIHUB_API_KEY = "<密钥>"
go run ./cmd/eval -scan datasets/static-v1/images -repeat 1 -out eval-runs/runs.jsonl -report eval-runs/report.md
```

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `-scan` | `datasets/static-v1/images` | 扫描 `<label>_pos` / `_neg` |
| `-manifest` | 空 | 指定后优先使用该 jsonl，不再扫描目录 |
| `-repeat` | `1` | 每个样本重复次数，用来看稳定性 |
| `-hit` | `0.8` | `noul` 大于等于该值记为 HIT |
| `-not-hit` | `0.2` | `noul` 小于等于该值记为 NOT_HIT |
| `-sweep` | 关闭 | 输出分标签阈值扫描 |
| `-out` / `-report` | `eval-runs/runs.jsonl`、`eval-runs/report.md` | 逐次结果和汇总 |

介于两个阈值之间记为 `UNCERTAIN`，没拿到分数记为 `UNKNOWN`。

## 测试

```powershell
cd src
go test ./...
```

带 `live` 的测试会访问 Hub，需要 `AIHUB_API_KEY`。

## 方案

建设计划和分阶段方案在 `docs/`：

- `Banner创意违规识别_JEV递进式建设计划.md`
- `Banner创意违规识别_Phase0测试方案.md` 到 `Phase3统一结果方案.md`
- `素材获取模块方案.md`
