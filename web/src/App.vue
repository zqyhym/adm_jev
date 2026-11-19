<script setup lang="ts">
import { computed, ref } from "vue";
import { DataAnalysis, DocumentChecked, View as ViewIcon } from "@element-plus/icons-vue";
import PreviewView from "./views/PreviewView.vue";
import ReportView from "./views/ReportView.vue";
import ReviewView from "./views/ReviewView.vue";

type ViewId = "preview" | "review" | "report";

const initial = window.location.hash.replace(/^#\/?/, "");
const activeView = ref<ViewId>(initial === "review" || initial === "report" ? initial : "preview");
const viewMeta: Record<ViewId, { title: string; description: string; scope: [string, string] }> = {
  preview: {
    title: "从原始内容，到创意画面。",
    description: "粘贴广告响应，检查画面、素材与拦截记录。",
    scope: ["面向已获取的广告响应", "仅解析提交内容"],
  },
  review: {
    title: "从素材下载，到审核结果。",
    description: "下载创意图片，查看 JEV 违规标签与 URL 协议结论。",
    scope: ["自动审核", "结果写入审核记录"],
  },
  report: {
    title: "从每次提交，到审核记录。",
    description: "按时间、状态和创意查看已落库的审核结果。",
    scope: ["每次输入一条记录", "含失败与告警"],
  },
};
const activeMeta = computed(() => viewMeta[activeView.value]);

function selectView(index: string): void {
  const next: ViewId = index === "review" || index === "report" ? index : "preview";
  activeView.value = next;
  history.replaceState(null, "", `#/${next}`);
}
</script>

<template>
  <div class="console-shell">
    <aside class="sidebar">
      <div class="brand sidebar-brand">
        <div class="brand-mark" aria-hidden="true">
          <svg viewBox="0 0 28 28" fill="none">
            <path d="m10 8-6 6 6 6m8-12 6 6-6 6M16 5l-4 18" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
        </div>
        <div>
          <p class="eyebrow">ADM / CREATIVE CONSOLE</p>
          <h1>ADM 创意审核台</h1>
        </div>
      </div>

      <nav class="sidebar-nav" aria-label="功能导航">
        <el-menu :default-active="activeView" class="sidebar-menu" @select="selectView">
          <el-menu-item index="preview">
            <el-icon><ViewIcon /></el-icon>
            <template #title>
              <span class="nav-copy">
                <strong>素材预览</strong>
              </span>
            </template>
          </el-menu-item>
          <el-menu-item index="review">
            <el-icon><DocumentChecked /></el-icon>
            <template #title>
              <span class="nav-copy">
                <strong>素材审核</strong>
              </span>
            </template>
          </el-menu-item>
          <el-menu-item index="report">
            <el-icon><DataAnalysis /></el-icon>
            <template #title>
              <span class="nav-copy">
                <strong>审核报表</strong>
              </span>
            </template>
          </el-menu-item>
        </el-menu>
      </nav>

      <div class="sidebar-footer">
        <span class="footer-status"><span class="status-dot" aria-hidden="true"></span>静态解析 · 内网审核</span>
      </div>
    </aside>

    <main class="console-main">
      <header class="console-header">
        <div class="console-header-inner">
          <div class="workspace-heading">
            <div>
              <h2>{{ activeMeta.title }}</h2>
              <p class="workspace-description">{{ activeMeta.description }}</p>
            </div>
            <p class="workspace-scope">{{ activeMeta.scope[0] }}<span aria-hidden="true"> / </span>{{ activeMeta.scope[1] }}</p>
          </div>
        </div>
      </header>
      <div class="console-scroll">
        <PreviewView v-show="activeView === 'preview'" />
        <ReviewView v-show="activeView === 'review'" />
        <ReportView v-show="activeView === 'report'" />
      </div>
    </main>
  </div>
</template>
