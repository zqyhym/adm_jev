<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import PreviewFrame from "./PreviewFrame.vue";
import { sampleInput } from "./sample";
import type { ImageAsset } from "./types";
import { usePreview } from "./usePreview";

type TabId = "preview" | "assets" | "texts" | "blocked";

const input = ref("");
const inputElement = ref<HTMLTextAreaElement | null>(null);
const activeTab = ref<TabId>("preview");
const inputNotice = ref("");
const failedImages = ref(new Set<string>());
const { snapshot, loading, error, submittedInput, run, addBlocked, clear } = usePreview();
const result = computed(() => snapshot.value?.data ?? null);
const assets = computed(() => (result.value?.images ?? []).filter((asset) => !asset.tracker));
const texts = computed(() => result.value?.texts ?? []);
const blocked = computed(() => snapshot.value?.blocked ?? []);
const inputChanged = computed(() => snapshot.value !== null && input.value !== snapshot.value.input);
const canSubmit = computed(() => Boolean(input.value.trim()) && !(loading.value && input.value === submittedInput.value));
const tabs = computed<{ id: TabId; label: string; count?: number }[]>(() => [
  { id: "preview", label: "画面" },
  { id: "assets", label: "素材", count: snapshot.value ? assets.value.length : undefined },
  { id: "texts", label: "文本", count: snapshot.value ? texts.value.length : undefined },
  { id: "blocked", label: "拦截记录", count: snapshot.value ? blocked.value.length : undefined },
]);

const status = computed(() => {
  if (loading.value) return "正在解析";
  if (error.value) return "请求未成功";
  if (inputChanged.value) return "输入已修改";
  return snapshot.value ? "解析完成" : "等待输入";
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
  void nextTick(() => inputElement.value?.focus());
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
  inputElement.value?.focus();
}

function navigateTabs(event: KeyboardEvent, index: number): void {
  let next = index;
  if (event.key === "ArrowRight") next = (index + 1) % tabs.value.length;
  else if (event.key === "ArrowLeft") next = (index - 1 + tabs.value.length) % tabs.value.length;
  else if (event.key === "Home") next = 0;
  else if (event.key === "End") next = tabs.value.length - 1;
  else return;
  event.preventDefault();
  activeTab.value = tabs.value[next].id;
  void nextTick(() => document.getElementById(`tab-${activeTab.value}`)?.focus());
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="topbar-inner">
        <div class="brand">
          <div class="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 28 28" fill="none">
              <path d="m10 8-6 6 6 6m8-12 6 6-6 6M16 5l-4 18" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" />
            </svg>
          </div>
          <div>
            <p class="eyebrow">ADM / CREATIVE INSPECTOR</p>
            <h1>创意预览工作台</h1>
          </div>
        </div>
        <div class="topbar-note">
          <span class="status-dot"></span>
          静态解析 · 按需预览
        </div>
      </div>
    </header>

    <main class="workspace">
      <div class="workspace-heading">
        <div>
          <p class="eyebrow">CREATIVE WORKSPACE</p>
          <h2>从原始内容，到创意画面。</h2>
          <p class="workspace-description">粘贴广告响应，检查画面、素材与拦截记录。</p>
        </div>
        <p class="workspace-scope">面向已获取的广告响应<span aria-hidden="true"> / </span>仅解析提交内容</p>
      </div>

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
            <button class="button button-subtle example-button" type="button" @click="fillSample">
              <svg class="icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                <path d="M7 3H4.5A1.5 1.5 0 0 0 3 4.5v11A1.5 1.5 0 0 0 4.5 17h11a1.5 1.5 0 0 0 1.5-1.5V13M12 3h5v5m0-5-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              填入示例
            </button>
          </div>

          <form class="input-form" @submit.prevent="submit">
            <div class="editor">
              <div class="editor-toolbar">
                <label for="source-input">源内容</label>
                <span class="editor-language">JSON · HTML · XML</span>
              </div>
              <textarea
                id="source-input"
                ref="inputElement"
                v-model="input"
                class="source-input"
                spellcheck="false"
                autocapitalize="off"
                autocomplete="off"
                autocorrect="off"
                :aria-describedby="error ? 'input-help preview-error' : 'input-help'"
                placeholder="在这里粘贴 Bid Response JSON 或裸 ADM…&#10;&#10;保留原始内容即可，无需手动格式化。&#10;也可以先填入一份示例。"
                @input="inputNotice = ''"
              ></textarea>
              <div class="editor-footer">
                <span><span class="tiny-dot"></span>原文输入 · 不自动执行</span>
                <span class="character-count">{{ input.length.toLocaleString() }} 字符</span>
              </div>
            </div>
            <p id="input-help" class="input-help">支持完整 Bid Response 或裸 ADM。示例只填入编辑区，不会自动发起预览。</p>

            <div v-if="error" id="preview-error" class="error-message" role="alert">
              <svg class="icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                <circle cx="10" cy="10" r="7" stroke="currentColor" stroke-width="1.5" />
                <path d="M10 6v4m0 3v.1" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
              </svg>
              <div>
                <strong>未能完成本次预览</strong>
                <p>{{ error }}</p>
                <p v-if="submittedInput !== input">此错误来自上次提交的内容，当前输入尚未提交。</p>
              </div>
            </div>

            <div class="input-actions">
              <button class="button button-secondary" type="button" @click="clearWorkspace">
                <svg class="icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                  <path d="M3.5 5.5h13M8 3h4m-7 2.5.8 10A1.5 1.5 0 0 0 7.3 17h5.4a1.5 1.5 0 0 0 1.5-1.5l.8-10M8 8.5v5m4-5v5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
                </svg>
                清空
              </button>
              <button class="button button-primary" type="submit" :disabled="!canSubmit" :aria-busy="loading">
                <span v-if="loading" class="spinner" aria-hidden="true"></span>
                <svg v-else class="icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                  <path d="m7 4 9 6-9 6V4Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" />
                </svg>
                {{ loading ? (canSubmit ? '重新预览' : '正在解析…') : '预览' }}
              </button>
            </div>
            <p class="input-notice" role="status" aria-live="polite">{{ inputNotice || '仅在点击「预览」时提交内容。' }}</p>
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
            <span
              class="result-status"
              :class="{ 'is-loading': loading, 'is-error': error && !loading, 'is-ready': snapshot && !loading && !error && !inputChanged }"
              role="status"
              aria-live="polite"
              aria-atomic="true"
            >
              <span v-if="loading" class="spinner" aria-hidden="true"></span>
              <span v-else class="status-dot" aria-hidden="true"></span>
              {{ status }}
            </span>
          </div>

          <dl class="metadata">
            <div class="metadata-item">
              <dt>广告格式 <span>FORMAT</span></dt>
              <dd :class="{ 'is-placeholder': !result }">
                <span v-if="result" class="format-value">{{ formats[result.format] || result.format || '未提供' }}</span>
                <span v-else>—</span>
              </dd>
            </div>
            <div class="metadata-item">
              <dt>画面尺寸 <span>DIMENSIONS</span></dt>
              <dd class="mono" :class="{ 'is-placeholder': !result }">{{ result ? `${result.width} × ${result.height}` : '—' }}<span v-if="result" class="unit">px</span></dd>
            </div>
            <div class="metadata-item">
              <dt>创意标识 <span>CRID</span></dt>
              <dd class="mono" :class="{ 'is-placeholder': !result }">{{ result ? result.crid || '未提供' : '—' }}</dd>
            </div>
            <div class="metadata-item">
              <dt>广告主域名 <span>DOMAIN</span></dt>
              <dd :class="{ 'is-placeholder': !result }">{{ result ? result.adomain || '未提供' : '—' }}</dd>
            </div>
          </dl>

          <p v-if="resultNotice" class="result-notice" role="status">{{ resultNotice }}</p>

          <div class="result-tabs" role="tablist" aria-label="预览结果分类">
            <button
              v-for="(tab, index) in tabs"
              :id="`tab-${tab.id}`"
              :key="tab.id"
              class="tab-button"
              type="button"
              role="tab"
              :aria-selected="activeTab === tab.id"
              :aria-controls="`panel-${tab.id}`"
              :tabindex="activeTab === tab.id ? 0 : -1"
              @click="activeTab = tab.id"
              @keydown="navigateTabs($event, index)"
            >
              {{ tab.label }}
              <span v-if="tab.count !== undefined" class="tab-count">{{ tab.count }}</span>
            </button>
          </div>

          <div class="result-content">
            <div id="panel-preview" v-show="activeTab === 'preview'" role="tabpanel" aria-labelledby="tab-preview" tabindex="0">
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
                <div class="empty-state primary-empty">
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
                  <h3>{{ loading ? '正在解析创意' : '等待一份创意' }}</h3>
                  <p>{{ loading ? '响应就绪后，将在这里加载预览画面。' : '在输入区粘贴内容并点击「预览」，' }}<br v-if="!loading" /><span v-if="!loading">让原始代码变成可检查的画面。</span></p>
                  <span class="empty-caption">{{ loading ? 'PARSING CONTENT' : 'YOUR CREATIVE STARTS HERE' }}</span>
                </div>
              </div>
              <p class="panel-footnote">画面按实际广告尺寸展示，超出画布可滚动查看；未提供有效尺寸时以 1 px 占位。</p>
            </div>

            <div id="panel-assets" v-show="activeTab === 'assets'" role="tabpanel" aria-labelledby="tab-assets" tabindex="0">
              <div class="tab-intro"><h3>提取的素材</h3><p>仅展示非监测素材。媒体文件只显示信息，不自动加载或播放。</p></div>
              <div v-if="snapshot && assets.length" class="detail-scroll asset-list" tabindex="0" aria-label="素材列表">
                <figure v-for="(asset, index) in assets" :key="`${snapshot.id}-${index}`" class="asset-card">
                  <div class="asset-thumbnail">
                    <img
                      v-if="imageUrl(asset) && !failedImages.has(asset.url)"
                      :src="imageUrl(asset)"
                      :alt="`${sources[asset.source] || asset.source}提取的创意素材`"
                      loading="lazy"
                      decoding="async"
                      referrerpolicy="no-referrer"
                      @error="failedImages.add(asset.url)"
                    />
                    <div v-else class="asset-placeholder">
                      <svg class="icon" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                        <rect x="3" y="4" width="18" height="16" rx="3" stroke="currentColor" stroke-width="1.4" />
                        <path v-if="isMedia(asset)" d="m10 8 6 4-6 4V8Z" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round" />
                        <path v-else d="m4 16 5-5 4 4 3-3 5 5M16 8h.01" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
                      </svg>
                      <span>{{ isMedia(asset) ? '媒体资源 · 不自动播放' : failedImages.has(asset.url) ? '图片暂时无法加载' : '资源地址 · 仅文本展示' }}</span>
                    </div>
                  </div>
                  <figcaption>
                    <div class="asset-heading"><span class="asset-index">{{ String(index + 1).padStart(2, '0') }}</span><strong>{{ sources[asset.source] || asset.source || '素材' }}</strong><span class="asset-type">{{ isMedia(asset) ? 'MEDIA' : 'IMAGE' }}</span></div>
                    <code class="resource-url">{{ asset.url }}</code>
                  </figcaption>
                </figure>
              </div>
              <div v-else class="empty-state detail-empty">
                <svg class="empty-icon" viewBox="0 0 32 32" fill="none" aria-hidden="true"><rect x="5" y="5" width="22" height="22" rx="4" stroke="currentColor" stroke-width="1.5" /><path d="m6 22 7-8 5 5 4-4 5 6M22 11h.01" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" /></svg>
                <h3>{{ snapshot ? '未提取到可展示的素材' : '素材将在这里归集' }}</h3>
                <p>{{ snapshot ? '当前内容没有非监测图片或媒体资源。' : '完成一次预览后，查看图片、来源与资源地址。' }}</p>
              </div>
            </div>

            <div id="panel-texts" v-show="activeTab === 'texts'" role="tabpanel" aria-labelledby="tab-texts" tabindex="0">
              <div class="tab-intro"><h3>可见文字</h3><p>从提交内容中静态提取，不代表脚本执行后的全部文字。</p></div>
              <ol v-if="texts.length" class="detail-scroll text-list" tabindex="0" aria-label="提取文字列表">
                <li v-for="(text, index) in texts" :key="index"><span class="list-index">{{ String(index + 1).padStart(2, '0') }}</span><p>{{ text }}</p></li>
              </ol>
              <div v-else class="empty-state detail-empty">
                <svg class="empty-icon" viewBox="0 0 32 32" fill="none" aria-hidden="true"><path d="M7 6h18M16 6v20m-5 0h10M7 6v4m18-4v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" /></svg>
                <h3>{{ snapshot ? '没有提取到可见文字' : '文字内容，一目了然' }}</h3>
                <p>{{ snapshot ? '创意可能主要由图片、视频或动态脚本构成。' : '预览后，在这里检查静态解析得到的文字。' }}</p>
              </div>
            </div>

            <div id="panel-blocked" v-show="activeTab === 'blocked'" role="tabpanel" aria-labelledby="tab-blocked" tabindex="0">
              <div class="tab-intro"><h3>已拦截的地址</h3><p>合并静态解析与当前预览上报的记录，相同地址只显示一次。</p></div>
              <ol v-if="blocked.length" class="detail-scroll blocked-list" tabindex="0" aria-label="已拦截地址列表">
                <li v-for="(url, index) in blocked" :key="url"><span class="list-index">{{ String(index + 1).padStart(2, '0') }}</span><div><span class="blocked-label">已拦截</span><code class="resource-url">{{ url }}</code></div></li>
              </ol>
              <div v-else class="empty-state detail-empty">
                <svg class="empty-icon" viewBox="0 0 32 32" fill="none" aria-hidden="true"><path d="m16 4 10 4v8c0 6-10 12-10 12S6 22 6 16V8l10-4Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" /><path d="M11 16h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" /></svg>
                <h3>{{ snapshot ? '暂未记录到拦截地址' : '拦截记录将在这里显示' }}</h3>
                <p>{{ snapshot ? '没有记录不代表没有外部请求或内容完全安全。' : '预览时记录已识别的拦截地址，不发起额外探测。' }}</p>
              </div>
            </div>
          </div>
        </section>
      </div>

      <footer class="workspace-footer">
        <svg class="icon" viewBox="0 0 20 20" fill="none" aria-hidden="true"><path d="m10 2 6 2.5V10c0 4-6 7-6 7s-6-3-6-7V4.5L10 2Z" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" /><path d="M10 6v4m0 3v.1" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
        <p>预览可能加载第三方资源，拦截非完整安全沙箱。请仅预览你信任的内容。</p>
        <span class="footer-label">ADM INSPECTOR</span>
      </footer>
    </main>
  </div>
</template>
