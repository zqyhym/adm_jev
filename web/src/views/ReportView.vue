<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { DocumentAdd, Search } from "@element-plus/icons-vue";
import type { ReviewRecord } from "../types";
import { reasonText, recordReasons } from "../reviewReasons";
import { useReviewReport } from "../useReviewReport";

const statusLabels: Record<string, string> = { success: "成功", warning: "告警", failed: "失败" };
const statusTypes: Record<string, "success" | "warning" | "danger" | "info"> = {
  success: "success",
  warning: "warning",
  failed: "danger",
};
const groupLabels: Record<string, string> = { creative: "创意资源", notice: "通知地址", unresolved: "无法解析" };
const parseLabels: Record<string, string> = { OK: "完整", PARTIAL: "部分", FAILED: "失败" };

function formatDay(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

const today = new Date();
const start = new Date(today);
start.setDate(today.getDate() - 6);
const range = ref<[string, string]>([formatDay(start), formatDay(today)]);
const status = ref("");
const keyword = ref("");
const page = ref(1);
const pageSize = ref(20);
const formError = ref("");
const drawer = ref(false);
const { report, detail, loading, detailLoading, error, detailError, load, open, clearDetail } = useReviewReport();

const metrics = computed(() => {
  const data = report.value;
  return [
    { label: "审核次数", value: data ? data.total : "—", hint: "当前筛选" },
    { label: "成功", value: data ? data.success : "—", hint: "完成且无告警" },
    { label: "告警", value: data ? data.warning : "—", hint: "有审核告警" },
    { label: "失败", value: data ? data.failed : "—", hint: "请求未完成" },
    { label: "Unsecured 命中", value: data ? data.hit : "—", hint: "明文资源" },
  ];
});
const records = computed(() => report.value?.records ?? []);
const reasons = computed(() => report.value?.reasons ?? []);
const notice = computed(() => formError.value || error.value);

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function joinValues(values: readonly string[]): string {
  return values.length ? values.join("、") : "—";
}

function rowReasonText(row: ReviewRecord): string {
  const codes = recordReasons(row.labels);
  return codes.length ? codes.map(reasonText).join("、") : "—";
}

function reload(): void {
  if (!range.value || range.value.length !== 2 || !range.value[0] || !range.value[1]) {
    formError.value = "请选择日期范围。";
    return;
  }
  formError.value = "";
  void load({
    from: range.value[0],
    to: range.value[1],
    status: status.value,
    keyword: keyword.value,
    page: page.value,
    pageSize: pageSize.value,
  });
}

function search(): void {
  page.value = 1;
  reload();
}

function changePage(next: number): void {
  if (next === page.value) return;
  page.value = next;
  reload();
}

function changeSize(next: number): void {
  pageSize.value = next;
  page.value = 1;
  reload();
}

function openRecord(id: string): void {
  drawer.value = true;
  void open(id);
}

function openRow(row: ReviewRecord): void {
  openRecord(row.id);
}

onMounted(reload);
</script>

<template>
  <div class="workspace report-view">
    <section class="panel report-panel" aria-label="审核报表">
      <div class="report-body">
        <form class="report-filters" @submit.prevent="search">
          <el-date-picker
            v-model="range"
            class="report-range"
            type="daterange"
            value-format="YYYY-MM-DD"
            range-separator="至"
            start-placeholder="开始日期"
            end-placeholder="结束日期"
            :clearable="false"
          />
          <el-select v-model="status" class="report-status" clearable placeholder="全部状态">
            <el-option label="成功" value="success" />
            <el-option label="告警" value="warning" />
            <el-option label="失败" value="failed" />
          </el-select>
          <el-input v-model="keyword" class="report-keyword" clearable placeholder="创意 ID 或广告主" @keyup.enter="search" />
          <el-button type="primary" :icon="Search" :loading="loading" native-type="submit">查询</el-button>
        </form>

        <el-alert v-if="notice" class="form-alert" type="error" :closable="false" show-icon :title="notice" />

        <div class="review-metrics">
          <div v-for="metric in metrics" :key="metric.label" class="metric-card">
            <span class="metric-label">{{ metric.label }}</span>
            <strong class="metric-value">{{ metric.value }}</strong>
            <small>{{ metric.hint }}</small>
          </div>
        </div>

        <div v-if="reasons.length">
          <h3 class="report-section-title">UNKNOWN 归因</h3>
          <div class="report-reasons">
            <el-tag v-for="item in reasons" :key="item.reason" size="small" type="info" effect="plain">
              {{ reasonText(item.reason) }} · {{ item.count }}
            </el-tag>
          </div>
        </div>

        <div>
          <h3 class="report-section-title">审核记录</h3>
          <el-table
            :data="records"
            row-key="id"
            class="review-table"
            max-height="480"
            @row-click="openRow"
          >
            <el-table-column label="时间" min-width="168">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="statusTypes[row.status] || 'info'">{{ statusLabels[row.status] || row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="创意 ID" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">{{ joinValues(row.crids) }}</template>
            </el-table-column>
            <el-table-column label="广告主" min-width="140" show-overflow-tooltip>
              <template #default="{ row }">{{ joinValues(row.adomains) }}</template>
            </el-table-column>
            <el-table-column label="协议" width="110">
              <template #default="{ row }">{{ row.unsecuredStatus || "—" }}</template>
            </el-table-column>
            <el-table-column label="未定原因" min-width="160" show-overflow-tooltip>
              <template #default="{ row }">{{ rowReasonText(row) }}</template>
            </el-table-column>
            <el-table-column label="说明" min-width="180" show-overflow-tooltip>
              <template #default="{ row }">{{ row.error || row.reviewError || "—" }}</template>
            </el-table-column>
            <el-table-column label="模型" min-width="120" show-overflow-tooltip>
              <template #default="{ row }">{{ row.model || "—" }}</template>
            </el-table-column>
          </el-table>
          <div class="report-pagination">
            <el-pagination
              :current-page="page"
              :page-size="pageSize"
              :total="report?.total ?? 0"
              :page-sizes="[20, 50, 100]"
              layout="total, sizes, prev, pager, next"
              @current-change="changePage"
              @size-change="changeSize"
            />
          </div>
        </div>
      </div>
    </section>

    <footer class="workspace-footer">
      <el-icon><DocumentAdd /></el-icon>
      <p>每条记录对应一次 ADM 输入。失败请求同样保留；noul 分数未经校准，不构成最终违规结论。</p>
      <span class="footer-label">REVIEW REPORT</span>
    </footer>

    <el-drawer v-model="drawer" title="审核详情" size="720px" @closed="clearDetail">
      <div v-loading="detailLoading" class="report-drawer">
        <el-alert v-if="detailError" type="error" :closable="false" show-icon :title="detailError" />
        <template v-else-if="detail">
          <el-descriptions :column="2" size="small" border>
            <el-descriptions-item label="状态">
              <el-tag size="small" :type="statusTypes[detail.status] || 'info'">{{ statusLabels[detail.status] || detail.status }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="时间">{{ formatTime(detail.createdAt) }}</el-descriptions-item>
            <el-descriptions-item label="耗时">{{ (detail.elapsedMs / 1000).toFixed(1) }} 秒</el-descriptions-item>
            <el-descriptions-item label="模型">{{ detail.model || "—" }}</el-descriptions-item>
            <el-descriptions-item label="创意 ID">{{ joinValues(detail.crids) }}</el-descriptions-item>
            <el-descriptions-item label="广告主">{{ joinValues(detail.adomains) }}</el-descriptions-item>
            <el-descriptions-item label="协议结论">{{ detail.unsecuredStatus || "—" }}</el-descriptions-item>
            <el-descriptions-item label="已评分标签">{{ detail.scoredCount }}</el-descriptions-item>
          </el-descriptions>

          <el-alert
            v-if="detail.error || detail.reviewError"
            class="report-detail-alert"
            :type="detail.error ? 'error' : 'warning'"
            :closable="false"
            show-icon
            :title="detail.error || detail.reviewError"
          />

          <div class="report-drawer-section">
            <h3>原始输入</h3>
            <el-input :model-value="detail.input || ''" type="textarea" :rows="8" readonly resize="vertical" />
          </div>

          <template v-if="detail.result">
            <div class="report-drawer-section">
              <h3>竞价</h3>
              <el-table :data="detail.result.bids" class="simple-table" max-height="240">
                <el-table-column prop="crid" label="创意 ID" min-width="120" show-overflow-tooltip />
                <el-table-column prop="adomain" label="广告主" min-width="140" show-overflow-tooltip />
                <el-table-column label="尺寸" width="110">
                  <template #default="{ row }">{{ row.w }} × {{ row.h }}</template>
                </el-table-column>
                <el-table-column label="解析" width="90">
                  <template #default="{ row }">{{ parseLabels[row.parseStatus] || row.parseStatus }}</template>
                </el-table-column>
              </el-table>
            </div>
            <div class="report-drawer-section">
              <h3>标签</h3>
              <el-table :data="detail.result.labels" class="simple-table" max-height="280">
                <el-table-column prop="label" label="标签" min-width="160" />
                <el-table-column prop="sourceId" label="来源" min-width="140" show-overflow-tooltip />
                <el-table-column prop="status" label="状态" width="100" />
                <el-table-column label="原因" min-width="150" show-overflow-tooltip>
                  <template #default="{ row }">{{ reasonText(row.reason) }}</template>
                </el-table-column>
                <el-table-column label="noul" width="90">
                  <template #default="{ row }">{{ row.noul === null ? "—" : row.noul.toFixed(3) }}</template>
                </el-table-column>
              </el-table>
            </div>
            <div class="report-drawer-section">
              <h3>URL 协议</h3>
              <el-table :data="detail.result.urls" class="simple-table" max-height="240">
                <el-table-column label="分组" width="110">
                  <template #default="{ row }">{{ groupLabels[row.group] || row.group }}</template>
                </el-table-column>
                <el-table-column prop="scheme" label="协议" width="90" />
                <el-table-column label="明文" width="80">
                  <template #default="{ row }">{{ row.unsecured ? "http" : "—" }}</template>
                </el-table-column>
                <el-table-column prop="url" label="地址" min-width="220" show-overflow-tooltip />
              </el-table>
            </div>
          </template>
        </template>
      </div>
    </el-drawer>
  </div>
</template>
