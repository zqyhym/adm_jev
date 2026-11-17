<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { CaretRight, Delete, DocumentAdd, Loading, Picture, VideoPlay } from "@element-plus/icons-vue";
import PreviewFrame from "../PreviewFrame.vue";
import { sampleInput } from "../sample";
import type { ImageAsset } from "../types";
import { usePreview } from "../usePreview";

type TabId = "preview" | "assets" | "texts" | "blocked";

const input = ref("");
const editorElement = ref<HTMLElement | null>(null);
const activeTab = ref<TabId>("preview");
const inputNotice = ref("");
const failedImages = ref(new Set<string>());
const { snapshot, loading, error, submittedInput, run, addBlocked, clear } = usePreview();
const result = computed(() => snapshot.value?.data ?? null);
const assets = computed(() => (result.value?.images ?? []).filter((asset) => !asset.tracker));
const texts = computed(() => result.value?.texts ?? []);
const blocked = computed(() => snapshot.value?.blocked ?? []);
const textRows = computed(() => texts.value.map((text, index) => ({ index: index + 1, text })));
const blockedRows = computed(() => blocked.value.map((url, index) => ({ index: index + 1, url })));
const inputChanged = computed(() => snapshot.value !== null && input.value !== snapshot.value.input);
const canSubmit = computed(() => Boolean(input.value.trim()) && !(loading.value && input.value === submittedInput.value));

const status = computed(() => {
  if (loading.value) return "正在解析";
  if (error.value) return "请求未成功";
  if (inputChanged.value) return "输入已修改";
  return snapshot.value ? "解析完成" : "等待输入";
});

const statusType = computed<"info" | "danger" | "success">(() => {
  if (loading.value) return "info";
  if (error.value) return "danger";
  return snapshot.value && !inputChanged.value ? "success" : "info";
});

const resultNotice = computed(() => {
  if (loading.value) {
    const pending = input.value === submittedInput.value
      ? "正在解析新请求"
      : "正在解析上次提交的内容，编辑区修改尚未提交";
    return snapshot.value
      ? `${pending}；下方仍为上次成功结果，完成后将整体替换。`
      : input.value !== submittedInput.value ? `${pending}。可点击「重新预览」替换当前请求。` : "";
  }
  if (!snapshot.value) return "";
  if (error.value) return "本次请求未成功；下方保留上次成功结果，请核对提交内容。";
  if (inputChanged.value) return "编辑区已修改；下方仍为上次提交的结果，请重新预览以更新。";
  return "";
});

const formats: Record<string, string> = { banner: "横幅广告", native: "原生广告", vast: "视频广告" };
const sources: Record<string, string> = {
  tag: "标签图片",
  srcset: "响应式图片",
  background: "背景图片",
  token: "令牌素材",
  native: "原生素材",
  media: "媒体文件",
};

// Only thumbnail failures are reset here. Blocked records are initialized atomically in usePreview.
watch(() => snapshot.value?.id, () => { failedImages.value = new Set(); }, { flush: "sync" });

function focusEditor(): void {
  void nextTick(() => editorElement.value?.querySelector("textarea")?.focus());
}

function isMedia(asset: ImageAsset): boolean {
  return asset.source === "media" || /\.(mp4|m4v|webm|ogv|ogg|mov|m3u8|mpd)(?:[?#]|$)/i.test(asset.url);
}

function imageUrl(asset: ImageAsset): string {
  const raw = asset.url.trim();
  if (!raw || isMedia(asset)) return "";
  try {
    const url = new URL(raw, window.location.href);
    if (url.protocol === "http:" || url.protocol === "https:") return url.href;
    if (/^data:image\/(?:png|jpe?g|gif|webp|avif|svg\+xml|bmp|x-icon)[;,]/i.test(raw)) return raw;
  } catch {
    // Unusable URLs are still shown as text, never inserted as HTML or links.
  }
  return "";
}

function fillSample(): void {
  input.value = sampleInput;
  inputNotice.value = "示例已填入，点击「预览」后才会执行。";
  focusEditor();
}

function submit(): void {
  if (!canSubmit.value) return;
  inputNotice.value = "";
  activeTab.value = "preview";
  void run(input.value);
}

async function clearWorkspace(): Promise<void> {
  clear();
  input.value = "";
  inputNotice.value = "已清空输入、预览与拦截记录。";
  activeTab.value = "preview";
  await nextTick();
  focusEditor();
}
</script>

<template>
  <div class="workspace">
    <div class="workbench">
      <section class="panel input-panel" aria-labelledby="input-heading">
        <div class="panel-heading">
          <div class="panel-title">
            <span class="step-number" aria-hidden="true">01</span>
            <div>
              <h2 id="input-heading">输入内容</h2>
              <p>Bid Response / ADM</p>
            </div>
          </div>
          <div class="panel-actions">
            <el-button link type="primary" class="example-button" :icon="DocumentAdd" @click="fillSample">填入示例</el-button>
            <el-button :icon="Delete" @click="clearWorkspace">清空</el-button>
            <el-button type="primary" :disabled="!canSubmit" :loading="loading" :icon="CaretRight" @click="submit">
              {{ loading ? (canSubmit ? "重新预览" : "正在解析…") : "预览" }}
            </el-button>
          </div>
        </div>

        <form class="input-form" @submit.prevent="submit">
          <div ref="editorElement" class="editor">
            <div class="editor-toolbar">
              <label for="source-input">源内容</label>
              <span class="editor-language">JSON · HTML · XML</span>
            </div>
            <el-input
              id="source-input"
              v-model="input"
              class="source-input"
              type="textarea"
              :rows="13"
              resize="vertical"
              spellcheck="false"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              :aria-describedby="error ? 'input-help preview-error' : 'input-help'"
              placeholder="在这里粘贴 Bid Response JSON 或裸 ADM…&#10;&#10;保留原始内容即可，无需手动格式化。&#10;也可以先填入一份示例。"
              @input="inputNotice = ''"
            />
            <div class="editor-footer">
              <span><span class="tiny-dot"></span>原文输入 · 不自动执行</span>
              <span class="character-count">{{ input.length.toLocaleString() }} 字符</span>
            </div>
          </div>
          <p id="input-help" class="input-help">支持完整 Bid Response 或裸 ADM。示例只填入编辑区，不会自动发起预览。</p>

          <el-alert
            v-if="error"
            id="preview-error"
            class="form-alert"
            type="error"
            :closable="false"
            show-icon
            title="未能完成本次预览"
          >
            <p>{{ error }}</p>
            <p v-if="submittedInput !== input">此错误来自上次提交的内容，当前输入尚未提交。</p>
          </el-alert>

          <p v-if="inputNotice" class="input-notice" role="status" aria-live="polite">{{ inputNotice }}</p>
        </form>
      </section>

      <section class="panel result-panel" aria-labelledby="result-heading">
        <div class="panel-heading">
          <div class="panel-title">
            <span class="step-number" aria-hidden="true">02</span>
            <div>
              <h2 id="result-heading">检查结果</h2>
              <p>Preview & inspection</p>
            </div>
          </div>
          <el-tag class="result-status" :type="statusType" effect="light" round aria-live="polite" aria-atomic="true">
            <el-icon v-if="loading" class="is-loading"><Loading /></el-icon>
            <span v-else class="status-dot" aria-hidden="true"></span>
            {{ status }}
          </el-tag>
        </div>

        <el-descriptions class="metadata-card" :column="2" border size="small">
          <el-descriptions-item>
            <template #label><span class="meta-label">广告格式 <small>FORMAT</small></span></template>
            <el-tag v-if="result" size="small" type="info">{{ formats[result.format] || result.format || "未提供" }}</el-tag>
            <span v-else class="is-placeholder">—</span>
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><span class="meta-label">画面尺寸 <small>DIMENSIONS</small></span></template>
            <span class="mono" :class="{ 'is-placeholder': !result }">{{ result ? `${result.width} × ${result.height}` : "—" }}</span>
            <span v-if="result" class="unit">px</span>
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><span class="meta-label">创意标识 <small>CRID</small></span></template>
            <span class="mono" :class="{ 'is-placeholder': !result }">{{ result ? result.crid || "未提供" : "—" }}</span>
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><span class="meta-label">广告主域名 <small>DOMAIN</small></span></template>
            <span :class="{ 'is-placeholder': !result }">{{ result ? result.adomain || "未提供" : "—" }}</span>
          </el-descriptions-item>
        </el-descriptions>

        <p v-if="resultNotice" class="result-notice" role="status">{{ resultNotice }}</p>

        <el-tabs v-model="activeTab" class="result-tabs">
          <el-tab-pane name="preview">
            <template #label>画面</template>
            <div class="canvas-toolbar">
              <span>预览画布</span>
              <span class="scale-label"><span aria-hidden="true">1:1</span> 原始尺寸</span>
            </div>
            <PreviewFrame
              v-if="snapshot"
              :key="snapshot.id"
              :snapshot-id="snapshot.id"
              :html="snapshot.data.previewHtml"
              :width="snapshot.data.width"
              :height="snapshot.data.height"
              @blocked="addBlocked"
            />
            <div v-else class="preview-canvas empty-canvas">
              <el-empty :description="loading ? '响应就绪后，将在这里加载预览画面。' : '在输入区粘贴内容并点击「预览」'">
                <template #image>
                  <div class="empty-illustration" aria-hidden="true">
                    <svg viewBox="0 0 100 84" fill="none">
                      <rect x="21" y="9" width="63" height="54" rx="6" stroke="currentColor" stroke-opacity=".25" stroke-dasharray="4 4" />
                      <rect x="12" y="19" width="66" height="53" rx="6" fill="var(--surface)" stroke="currentColor" stroke-width="1.5" />
                      <path d="M12 32h66" stroke="currentColor" stroke-opacity=".3" />
                      <circle cx="20" cy="26" r="1.5" fill="currentColor" />
                      <circle cx="26" cy="26" r="1.5" fill="currentColor" fill-opacity=".4" />
                      <path d="m35 43-8 8 8 8m20-16 8 8-8 8m-7-19-6 23" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" />
                      <path d="M85 66v10m-5-5h10M6 8v6M3 11h6" stroke="currentColor" stroke-opacity=".4" stroke-linecap="round" />
                    </svg>
                  </div>
                </template>
                <span class="empty-caption">{{ loading ? "PARSING CONTENT" : "YOUR CREATIVE STARTS HERE" }}</span>
              </el-empty>
            </div>
            <p class="panel-footnote">画面按实际广告尺寸展示，超出画布可滚动查看；未提供有效尺寸时以 1 px 占位。</p>
          </el-tab-pane>

          <el-tab-pane name="assets">
            <template #label>素材 <span v-if="snapshot" class="tab-count">{{ assets.length }}</span></template>
            <div class="tab-intro"><h3>提取的素材</h3><p>仅展示非监测素材。媒体文件只显示信息，不自动加载或播放。</p></div>
            <div v-if="snapshot && assets.length" class="detail-scroll asset-list" tabindex="0" aria-label="素材列表">
              <figure v-for="(asset, index) in assets" :key="`${snapshot.id}-${index}`" class="asset-card">
                <div class="asset-thumbnail">
                  <el-image
                    v-if="imageUrl(asset) && !failedImages.has(asset.url)"
                    :src="imageUrl(asset)"
                    :alt="`${sources[asset.source] || asset.source}提取的创意素材`"
                    class="asset-image"
                    fit="contain"
                    @error="failedImages.add(asset.url)"
                  >
                    <template #error>
                      <div class="asset-placeholder">
                        <el-icon><Picture /></el-icon>
                        <span>图片暂时无法加载</span>
                      </div>
                    </template>
                  </el-image>
                  <div v-else class="asset-placeholder">
                    <el-icon v-if="isMedia(asset)"><VideoPlay /></el-icon>
                    <el-icon v-else><Picture /></el-icon>
                    <span>{{ isMedia(asset) ? "媒体资源 · 不自动播放" : failedImages.has(asset.url) ? "图片暂时无法加载" : "资源地址 · 仅文本展示" }}</span>
                  </div>
                </div>
                <figcaption>
                  <div class="asset-heading"><span class="asset-index">{{ String(index + 1).padStart(2, "0") }}</span><strong>{{ sources[asset.source] || asset.source || "素材" }}</strong><span class="asset-type">{{ isMedia(asset) ? "MEDIA" : "IMAGE" }}</span></div>
                  <code class="resource-url">{{ asset.url }}</code>
                </figcaption>
              </figure>
            </div>
            <div v-else class="detail-empty">
              <el-empty :description="snapshot ? '当前内容没有非监测图片或媒体资源。' : '完成一次预览后，查看图片、来源与资源地址。'" />
            </div>
          </el-tab-pane>

          <el-tab-pane name="texts">
            <template #label>文本 <span v-if="snapshot" class="tab-count">{{ texts.length }}</span></template>
            <div class="tab-intro"><h3>可见文字</h3><p>从提交内容中静态提取，不代表脚本执行后的全部文字。</p></div>
            <el-table v-if="textRows.length" :data="textRows" class="simple-table" max-height="460">
              <el-table-column prop="index" label="#" width="64" />
              <el-table-column prop="text" label="文字内容" min-width="240" />
            </el-table>
            <div v-else class="detail-empty">
              <el-empty :description="snapshot ? '创意可能主要由图片、视频或动态脚本构成。' : '预览后，在这里检查静态解析得到的文字。'" />
            </div>
          </el-tab-pane>

          <el-tab-pane name="blocked">
            <template #label>拦截记录 <span v-if="snapshot" class="tab-count">{{ blocked.length }}</span></template>
            <div class="tab-intro"><h3>已拦截的地址</h3><p>合并静态解析与当前预览上报的记录，相同地址只显示一次。</p></div>
            <el-table v-if="blockedRows.length" :data="blockedRows" class="simple-table" max-height="460">
              <el-table-column prop="index" label="#" width="64" />
              <el-table-column label="状态" width="92">
                <template #default><el-tag size="small" type="danger">已拦截</el-tag></template>
              </el-table-column>
              <el-table-column prop="url" label="地址" min-width="280" show-overflow-tooltip />
            </el-table>
            <div v-else class="detail-empty">
              <el-empty :description="snapshot ? '没有记录不代表没有外部请求或内容完全安全。' : '预览时记录已识别的拦截地址，不发起额外探测。'" />
            </div>
          </el-tab-pane>
        </el-tabs>
      </section>
    </div>

    <footer class="workspace-footer">
      <el-icon><DocumentAdd /></el-icon>
      <p>预览可能加载第三方资源，拦截非完整安全沙箱。请仅预览你信任的内容。</p>
      <span class="footer-label">ADM INSPECTOR</span>
    </footer>
  </div>
</template>
