<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Delete, DocumentAdd, Loading, Picture, Search } from "@element-plus/icons-vue";
import { sampleReviewInput } from "../sample";
import { useReview } from "../useReview";
import type { LabelReview, ReviewAsset, UnsecuredVerdict, UrlReport } from "../types";

type TabId = "overview" | "assets" | "urls";
type AssetRow = ReviewAsset & { bidPath: string; crid: string };

const input = ref("");
const editorElement = ref<HTMLElement | null>(null);
const activeTab = ref<TabId>("overview");
const inputNotice = ref("");
const selectedId = ref("");
const failedSelected = ref(false);
const { snapshot, loading, error, submittedInput, run, clear } = useReview();

const result = computed(() => snapshot.value?.data ?? null);
const bids = computed(() => result.value?.bids ?? []);
const assets = computed<AssetRow[]>(() => bids.value.flatMap((bid) => bid.assets.map((asset) => ({
  ...asset,
  bidPath: bid.bidPath,
  crid: bid.crid,
}))));
const creativeImages = computed(() => assets.value.filter((asset) => asset.kind === "image" && asset.role === "creative"));
const labels = computed(() => (result.value?.labels ?? []).filter((label) => label.sourceId));
const labelMap = computed(() => {
  const map = new Map<string, LabelReview[]>();
  for (const label of labels.value) {
    const list = map.get(label.sourceId);
    if (list) list.push(label);
    else map.set(label.sourceId, [label]);
  }
  return map;
});
const urls = computed<UrlReport[]>(() => (result.value?.urls ?? []).map((item) => ({ ...item })));
const unsecured = computed<UnsecuredVerdict>(() => {
  const value = result.value?.unsecured;
  if (!value) return { status: "UNKNOWN", evidence: [], notices: [] };
  return {
    status: value.status,
    evidence: value.evidence.map((item) => ({ ...item })),
    notices: value.notices.map((item) => ({ ...item })),
  };
});
const scoredCount = computed(() => labels.value.filter((label) => label.status === "SCORED").length);
const unknownCount = computed(() => labels.value.filter((label) => label.status === "UNKNOWN").length);
const obtainedCount = computed(() => creativeImages.value.filter((asset) => asset.obtained).length);
const selectedAsset = computed(() => creativeImages.value.find((asset) => asset.sourceId === selectedId.value) ?? null);
const selectedLabels = computed(() => labelMap.value.get(selectedId.value) ?? []);
const elapsed = computed(() => snapshot.value ? `${(snapshot.value.elapsedMs / 1000).toFixed(1)}` : "—");
const metrics = computed(() => [
  { label: "BID", value: bids.value.length, hint: "个竞价记录" },
  { label: "创意图片", value: creativeImages.value.length, hint: "张主体素材" },
  { label: "已取得", value: obtainedCount.value, hint: "下载并通过校验" },
  { label: "已评分标签", value: scoredCount.value, hint: "SCORED" },
  { label: "未定标签", value: unknownCount.value, hint: "UNKNOWN" },
  { label: "耗时", value: elapsed.value, hint: "秒 · 客户端等待" },
]);
const inputChanged = computed(() => snapshot.value !== null && input.value !== snapshot.value.input);
const canSubmit = computed(() => Boolean(input.value.trim()) && !(loading.value && input.value === submittedInput.value));

const status = computed(() => {
  if (loading.value) return "正在审核";
  if (error.value) return "请求未成功";
  if (inputChanged.value) return "输入已修改";
  if (!snapshot.value) return "等待输入";
  return result.value?.reviewError ? "审核有告警" : "审核完成";
});

const statusType = computed<"info" | "danger" | "success" | "warning">(() => {
  if (loading.value) return "info";
  if (error.value) return "danger";
  if (!snapshot.value || inputChanged.value) return "info";
  return result.value?.reviewError ? "warning" : "success";
});

const resultNotice = computed(() => {
  if (loading.value) {
    const pending = input.value === submittedInput.value
      ? "正在审核新请求"
      : "正在审核上次提交的内容，编辑区修改尚未提交";
    return snapshot.value
      ? `${pending}；下方仍为上次成功结果，完成后将整体替换。`
      : input.value !== submittedInput.value ? `${pending}。可点击「重新审核」替换当前请求。` : "";
  }
  if (!snapshot.value) return "";
  if (error.value) return "本次审核未成功；下方保留上次成功结果，请核对提交内容。";
  if (inputChanged.value) return "编辑区已修改；下方仍为上次提交的审核结果，请重新审核以更新。";
  return "";
});

const reviewHint = computed(() => {
  const message = result.value?.reviewError ?? "";
  if (!message) return "";
  if (message.includes("AIHUB_API_KEY")) return "后端未配置 AIHUB_API_KEY，素材抽取、下载与协议规则仍可查看，视觉标签未生成。";
  if (message.includes("没有可送审的主体图片")) return "本次没有可送审的已取得创意图片，因此未调用 JEV。";
  return "审核未完全完成，以下为已返回的部分结果。";
});

const unsecuredText = computed(() => {
  if (unsecured.value.status === "HIT") return "创意资源中存在 http:// 明文地址。";
  if (unsecured.value.status === "NOT_HIT") return "创意资源均为可解析的 https:// 地址。";
  return "创意地址无法解析，或缺少可判断的创意资源。";
});

const kindLabels: Record<string, string> = { image: "图片", text: "文本", script: "脚本", iframe: "嵌入页", url: "链接" };
const roleLabels: Record<string, string> = { creative: "主体", tracker: "监测", dynamic: "动态" };
const groupLabels: Record<string, string> = { creative: "创意资源", notice: "通知地址", unresolved: "无法解析" };
const parseLabels: Record<string, string> = { OK: "完整", PARTIAL: "部分", FAILED: "失败" };

watch(() => snapshot.value?.id, () => {
  failedSelected.value = false;
  const first = creativeImages.value.find((asset) => asset.obtained) ?? creativeImages.value[0];
  selectedId.value = first?.sourceId ?? "";
}, { flush: "sync" });

function focusEditor(): void {
  void nextTick(() => editorElement.value?.querySelector("textarea")?.focus());
}

function fillSample(): void {
  input.value = sampleReviewInput;
  inputNotice.value = "审核示例已填入，点击「开始审核」后才会执行。";
  focusEditor();
}

function submit(): void {
  if (!canSubmit.value) return;
  inputNotice.value = "";
  activeTab.value = "overview";
  void run(input.value);
}

async function clearWorkspace(): Promise<void> {
  clear();
  input.value = "";
  inputNotice.value = "已清空输入与审核结果。";
  activeTab.value = "overview";
  selectedId.value = "";
  failedSelected.value = false;
  await nextTick();
  focusEditor();
}

function parseType(value: string): "success" | "warning" | "danger" {
  if (value === "OK") return "success";
  if (value === "FAILED") return "danger";
  return "warning";
}

function unsecuredType(value: string): "success" | "warning" | "danger" {
  if (value === "NOT_HIT") return "success";
  if (value === "HIT") return "danger";
  return "warning";
}

function isCreativeImage(asset: ReviewAsset | AssetRow): boolean {
  return asset.kind === "image" && asset.role === "creative";
}

function assetState(asset: AssetRow): string {
  if (!isCreativeImage(asset)) {
    if (asset.role === "tracker") return "监测素材";
    if (asset.kind === "text") return "文本";
    return "不下载";
  }
  return asset.obtained ? "已取得" : "下载失败";
}

function assetStateType(asset: AssetRow): "success" | "danger" | "info" {
  if (!isCreativeImage(asset)) return "info";
  return asset.obtained ? "success" : "danger";
}

function labelSummary(asset: AssetRow): string {
  if (!isCreativeImage(asset)) return "—";
  const list = labelMap.value.get(asset.sourceId) ?? [];
  if (!list.length) return "未送审";
  const scored = list.filter((label) => label.status === "SCORED").length;
  return `${scored}/${list.length} 已评分`;
}

function assetRowClass({ row }: { row: AssetRow }): string {
  if (row.sourceId === selectedId.value) return "is-selected";
  return isCreativeImage(row) ? "is-selectable" : "";
}

function selectAsset(row: AssetRow): void {
  if (!isCreativeImage(row)) return;
  selectedId.value = row.sourceId;
  failedSelected.value = false;
}

function shortHash(value?: string): string {
  return value ? `${value.slice(0, 12)}…` : "—";
}

function safeImageUrl(raw: string): string {
  const value = raw.trim();
  if (!value) return "";
  try {
    const url = new URL(value, window.location.href);
    if (url.protocol === "http:" || url.protocol === "https:") return url.href;
    if (/^data:image\/(?:png|jpe?g|gif|webp|avif|svg\+xml|bmp|x-icon)[;,]/i.test(value)) return value;
  } catch {
    // Unusable URLs are still shown as text, never inserted as HTML.
  }
  return "";
}
</script>

<template>
  <div class="workspace review-view">
    <div class="workbench review-workbench">
      <section class="panel input-panel review-input-panel" aria-labelledby="review-input-heading">
        <div class="panel-heading">
          <div class="panel-title">
            <span class="step-number" aria-hidden="true">01</span>
            <div>
              <h2 id="review-input-heading">待审内容</h2>
              <p>Bid Response / ADM</p>
            </div>
          </div>
          <div class="panel-actions">
            <el-button link type="primary" class="example-button" :icon="DocumentAdd" @click="fillSample">填入示例</el-button>
            <el-button :icon="Delete" @click="clearWorkspace">清空</el-button>
            <el-button type="primary" :disabled="!canSubmit" :loading="loading" :icon="Search" @click="submit">
              {{ loading ? (canSubmit ? "重新审核" : "正在审核…") : "开始审核" }}
            </el-button>
          </div>
        </div>

        <form class="input-form" @submit.prevent="submit">
          <div ref="editorElement" class="editor">
            <div class="editor-toolbar">
              <label for="review-input">源内容</label>
              <span class="editor-language">JSON · HTML · XML</span>
            </div>
            <el-input
              id="review-input"
              v-model="input"
              class="source-input"
              type="textarea"
              :rows="13"
              resize="vertical"
              spellcheck="false"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              :aria-describedby="error ? 'review-input-help review-error' : 'review-input-help'"
              placeholder="在这里粘贴 Bid Response JSON 或裸 ADM…&#10;&#10;审核会下载创意图片并调用内网 JEV。&#10;监测素材不会下载。"
              @input="inputNotice = ''"
            />
            <div class="editor-footer">
              <span><span class="tiny-dot"></span>审核会下载并送审 · 不自动执行</span>
              <span class="character-count">{{ input.length.toLocaleString() }} 字符</span>
            </div>
          </div>
          <p id="review-input-help" class="input-help">审核会下载创意图片并调用内网 JEV，可能耗时数秒到数分钟；监测素材不会下载。</p>

          <el-alert
            v-if="error"
            id="review-error"
            class="form-alert"
            type="error"
            :closable="false"
            show-icon
            title="未能完成本次审核"
          >
            <p>{{ error }}</p>
            <p v-if="submittedInput !== input">此错误来自上次提交的内容，当前输入尚未提交。</p>
          </el-alert>

          <p class="input-notice" role="status" aria-live="polite">{{ inputNotice || "仅在点击「开始审核」时提交内容。" }}</p>
        </form>
      </section>

      <section class="panel result-panel review-result-panel" aria-labelledby="review-result-heading">
        <div class="panel-heading">
          <div class="panel-title">
            <span class="step-number" aria-hidden="true">02</span>
            <div>
              <h2 id="review-result-heading">审核结果</h2>
              <p>Review & evidence</p>
            </div>
          </div>
          <el-tag class="result-status" :type="statusType" effect="light" round aria-live="polite" aria-atomic="true">
            <el-icon v-if="loading" class="is-loading"><Loading /></el-icon>
            <span v-else class="status-dot" aria-hidden="true"></span>
            {{ status }}
          </el-tag>
        </div>

        <template v-if="result">
          <div class="review-summary">
            <div class="review-metrics">
              <div v-for="metric in metrics" :key="metric.label" class="metric-card">
                <span class="metric-label">{{ metric.label }}</span>
                <strong class="metric-value">{{ metric.value }}</strong>
                <small>{{ metric.hint }}</small>
              </div>
            </div>
            <div class="review-verdict">
              <div class="verdict-head">
                <span class="verdict-label">UNSECURED REQUEST</span>
                <el-tag :type="unsecuredType(unsecured.status)" effect="dark" size="small">{{ unsecured.status }}</el-tag>
              </div>
              <p>{{ unsecuredText }}</p>
              <div class="verdict-meta">
                <span>模型 <strong>{{ result.model || "未提供" }}</strong></span>
                <span>耗时 <strong>{{ elapsed }} s</strong></span>
              </div>
            </div>
          </div>

          <el-alert
            v-if="result.reviewError"
            class="review-warning"
            type="warning"
            :closable="false"
            show-icon
            :title="reviewHint"
          >
            <p class="review-error-detail">{{ result.reviewError }}</p>
          </el-alert>

          <p v-if="resultNotice" class="result-notice" role="status">{{ resultNotice }}</p>

          <el-tabs v-model="activeTab" class="result-tabs review-tabs">
            <el-tab-pane name="overview">
              <template #label>总览</template>
              <div class="review-bids">
                <el-card v-for="bid in bids" :key="bid.bidPath" shadow="never" class="bid-card">
                  <template #header>
                    <div class="bid-card-head">
                      <strong>{{ bid.crid || "未提供 CRID" }}</strong>
                      <el-tag size="small" :type="parseType(bid.parseStatus)">{{ parseLabels[bid.parseStatus] || bid.parseStatus }}</el-tag>
                    </div>
                  </template>
                  <el-descriptions :column="2" size="small" border>
                    <el-descriptions-item label="Bid 路径"><span class="mono">{{ bid.bidPath }}</span></el-descriptions-item>
                    <el-descriptions-item label="广告主域名">{{ bid.adomain || "—" }}</el-descriptions-item>
                    <el-descriptions-item label="尺寸">{{ bid.w }} × {{ bid.h }} px</el-descriptions-item>
                    <el-descriptions-item label="Imp ID">{{ bid.impid || "—" }}</el-descriptions-item>
                    <el-descriptions-item label="ADM 类型" :span="2">
                      <span v-if="bid.admTypes.length" class="type-tags">
                        <el-tag v-for="type in bid.admTypes" :key="type" size="small" effect="plain">{{ type }}</el-tag>
                      </span>
                      <span v-else>—</span>
                    </el-descriptions-item>
                    <el-descriptions-item label="ADM Hash" :span="2">
                      <el-tooltip :content="bid.admHash" placement="top">
                        <span class="mono">{{ shortHash(bid.admHash) }}</span>
                      </el-tooltip>
                    </el-descriptions-item>
                    <el-descriptions-item v-if="bid.failure" label="失败原因" :span="2">{{ bid.failure }}</el-descriptions-item>
                  </el-descriptions>
                  <div class="bid-stats">
                    <span>素材 {{ bid.assets.length }}</span>
                    <span>创意图 {{ bid.assets.filter((asset) => asset.kind === "image" && asset.role === "creative").length }}</span>
                  </div>
                </el-card>
                <el-empty v-if="!bids.length" description="没有可展示的 bid" />
              </div>
            </el-tab-pane>

            <el-tab-pane name="assets">
              <template #label>素材与标签 <span v-if="snapshot" class="tab-count">{{ assets.length }}</span></template>
              <div class="tab-intro"><h3>素材清单与送审标签</h3><p>仅创意图片会被下载并送审；点击创意图行查看该图的十类标签。</p></div>
              <div class="review-asset-layout">
                <div class="asset-table-wrap">
                  <el-table
                    :data="assets"
                    row-key="sourceId"
                    max-height="540"
                    :row-class-name="assetRowClass"
                    class="review-table"
                    @row-click="selectAsset"
                  >
                    <el-table-column type="index" label="#" width="54" />
                    <el-table-column label="类型" width="86">
                      <template #default="{ row }"><el-tag size="small" effect="plain">{{ kindLabels[row.kind] || row.kind }}</el-tag></template>
                    </el-table-column>
                    <el-table-column label="角色" width="78">
                      <template #default="{ row }">{{ roleLabels[row.role] || row.role }}</template>
                    </el-table-column>
                    <el-table-column label="来源" min-width="150">
                      <template #default="{ row }"><span class="mono source-id">{{ row.sourceId }}</span></template>
                    </el-table-column>
                    <el-table-column label="地址 / 内容" min-width="240" show-overflow-tooltip>
                      <template #default="{ row }"><code class="resource-url">{{ row.value }}</code></template>
                    </el-table-column>
                    <el-table-column label="状态" width="112">
                      <template #default="{ row }"><el-tag size="small" :type="assetStateType(row)">{{ assetState(row) }}</el-tag></template>
                    </el-table-column>
                    <el-table-column label="标签" width="112">
                      <template #default="{ row }">{{ labelSummary(row) }}</template>
                    </el-table-column>
                    <el-table-column label="下载错误" min-width="160" show-overflow-tooltip>
                      <template #default="{ row }">{{ row.fetchError || "—" }}</template>
                    </el-table-column>
                  </el-table>
                </div>
                <aside class="label-panel">
                  <template v-if="selectedAsset">
                    <div class="label-asset">
                      <div class="label-thumb">
                        <el-image
                          v-if="safeImageUrl(selectedAsset.value) && !failedSelected"
                          :src="safeImageUrl(selectedAsset.value)"
                          fit="contain"
                          alt="送审创意图"
                          class="label-image"
                          @error="failedSelected = true"
                        >
                          <template #error>
                            <div class="label-thumb-empty">
                              <el-icon><Picture /></el-icon>
                            </div>
                          </template>
                        </el-image>
                        <div v-else class="label-thumb-empty">
                          <el-icon><Picture /></el-icon>
                          <span>{{ selectedAsset.obtained ? "原图无法在浏览器加载" : "图片未取得" }}</span>
                        </div>
                      </div>
                      <div class="label-asset-meta">
                        <strong>{{ selectedAsset.sourceId }}</strong>
                        <code class="resource-url">{{ selectedAsset.value }}</code>
                        <span class="hash-line">Hash {{ shortHash(selectedAsset.contentHash) }}</span>
                      </div>
                    </div>
                    <div v-if="selectedLabels.length" class="label-list">
                      <div v-for="label in selectedLabels" :key="label.label" class="label-row">
                        <div class="label-head">
                          <span>{{ label.label }}</span>
                          <el-tag size="small" :type="label.status === 'SCORED' ? 'success' : 'info'" effect="plain">{{ label.status === "SCORED" ? "已评分" : "未返回" }}</el-tag>
                          <strong>{{ label.noul === null ? "—" : label.noul.toFixed(3) }}</strong>
                        </div>
                        <el-progress v-if="label.noul !== null" :percentage="Math.round(label.noul * 100)" :show-text="false" :stroke-width="6" />
                        <div v-else class="label-bar-empty"></div>
                      </div>
                    </div>
                    <el-empty v-else :description="selectedAsset.obtained ? '该图片未送审或未返回标签' : '图片未取得，未送审'" :image-size="72" />
                    <p class="panel-footnote">noul 为模型原始分数，不代表命中结论；UNKNOWN 不补成未命中。</p>
                  </template>
                  <el-empty v-else description="选择一张创意图查看十类标签" />
                </aside>
              </div>
            </el-tab-pane>

            <el-tab-pane name="urls">
              <template #label>URL 协议 <span v-if="snapshot" class="tab-count">{{ urls.length }}</span></template>
              <div class="protocol-card">
                <div class="protocol-head">
                  <span class="verdict-label">UNSECURED REQUEST</span>
                  <el-tag :type="unsecuredType(unsecured.status)" effect="dark" size="small">{{ unsecured.status }}</el-tag>
                </div>
                <p>{{ unsecuredText }}</p>
                <div class="verdict-meta">
                  <span>证据 <strong>{{ unsecured.evidence.length }}</strong></span>
                  <span>通知地址 <strong>{{ unsecured.notices.length }}</strong></span>
                </div>
              </div>
              <el-table v-if="urls.length" :data="urls" class="review-table" max-height="420">
                <el-table-column type="index" label="#" width="54" />
                <el-table-column label="分组" width="110">
                  <template #default="{ row }"><el-tag size="small" effect="plain">{{ groupLabels[row.group] || row.group }}</el-tag></template>
                </el-table-column>
                <el-table-column label="协议" width="90">
                  <template #default="{ row }"><span class="mono">{{ row.scheme || "—" }}</span></template>
                </el-table-column>
                <el-table-column label="明文" width="90">
                  <template #default="{ row }"><el-tag v-if="row.unsecured" size="small" type="danger">http</el-tag><span v-else>—</span></template>
                </el-table-column>
                <el-table-column prop="url" label="地址" min-width="320" show-overflow-tooltip />
              </el-table>
              <el-empty v-else description="没有可展示的资源地址" />
            </el-tab-pane>
          </el-tabs>
        </template>

        <div v-else class="detail-empty review-empty">
          <el-empty :description="loading ? '正在抽取、下载并送审素材…' : '粘贴内容并点击「开始审核」，这里会展示抽取、下载与标签结果。'" />
        </div>
      </section>
    </div>

    <footer class="workspace-footer">
      <el-icon><DocumentAdd /></el-icon>
      <p>审核结果来自自动解析与内网模型；noul 分数未经校准，不构成最终违规结论。</p>
      <span class="footer-label">CREATIVE REVIEW</span>
    </footer>
  </div>
</template>
