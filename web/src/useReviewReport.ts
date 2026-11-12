import { onScopeDispose, readonly, ref, shallowRef } from "vue";
import { apiErrorMessage, http } from "./api/http";
import type { ReviewLabelCount, ReviewLabelStat, ReviewRecord, ReviewReport, ReviewReportQuery } from "./types";
import { isReviewResult } from "./useReview";

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isStringList(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function isNullableNumber(value: unknown): value is number | null {
  return value === null || isFiniteNumber(value);
}

function isLabelStat(value: unknown): value is ReviewLabelStat {
  if (!isObject(value)) return false;
  return isString(value.label) && isString(value.status) && isNullableNumber(value.noul);
}

function isLabelCount(value: unknown): value is ReviewLabelCount {
  if (!isObject(value)) return false;
  return isString(value.label) && isFiniteNumber(value.count);
}

function isReviewRecord(value: unknown): value is ReviewRecord {
  if (!isObject(value)) return false;
  if (!isString(value.id) || !isString(value.createdAt) || !isFiniteNumber(value.elapsedMs)) return false;
  if (!isString(value.status) || !isString(value.error) || !isString(value.reviewError) || !isString(value.model) || !isString(value.unsecuredStatus)) {
    return false;
  }
  if (!isStringList(value.crids) || !isStringList(value.adomains)) return false;
  if (!isFiniteNumber(value.bidCount) || !isFiniteNumber(value.creativeCount) || !isFiniteNumber(value.scoredCount)) return false;
  if (!Array.isArray(value.labels) || !value.labels.every(isLabelStat)) return false;
  if (value.input !== undefined && !isString(value.input)) return false;
  if (value.result !== undefined && value.result !== null && !isReviewResult(value.result)) return false;
  return true;
}

function isReviewReport(value: unknown): value is ReviewReport {
  if (!isObject(value)) return false;
  if (!isFiniteNumber(value.total) || !isFiniteNumber(value.success) || !isFiniteNumber(value.warning) || !isFiniteNumber(value.failed) || !isFiniteNumber(value.hit)) {
    return false;
  }
  if (!isFiniteNumber(value.page) || !isFiniteNumber(value.pageSize)) return false;
  if (!Array.isArray(value.labels) || !value.labels.every(isLabelCount)) return false;
  return Array.isArray(value.records) && value.records.every(isReviewRecord);
}

export function useReviewReport() {
  const report = shallowRef<ReviewReport | null>(null);
  const detail = shallowRef<ReviewRecord | null>(null);
  const loading = ref(false);
  const detailLoading = ref(false);
  const error = ref("");
  const detailError = ref("");
  let requestId = 0;
  let detailRequestId = 0;
  let controller: AbortController | null = null;
  let detailController: AbortController | null = null;

  async function load(query: ReviewReportQuery): Promise<void> {
    const id = ++requestId;
    controller?.abort();
    const current = new AbortController();
    controller = current;
    loading.value = true;
    error.value = "";
    try {
      const response = await http.get<unknown>("/api/review/records", {
        signal: current.signal,
        params: {
          from: query.from,
          to: query.to,
          page: query.page,
          pageSize: query.pageSize,
          ...(query.status ? { status: query.status } : {}),
          ...(query.keyword.trim() ? { q: query.keyword.trim() } : {}),
        },
      });
      if (!isReviewReport(response.data)) {
        throw new Error("审核报表返回的数据格式不符合约定，无法展示结果。");
      }
      if (id !== requestId || current.signal.aborted) return;
      report.value = response.data;
    } catch (cause) {
      if (id !== requestId || current.signal.aborted) return;
      error.value = apiErrorMessage(cause, "审核报表") || "审核报表查询失败，请稍后重试。";
    } finally {
      if (id === requestId) {
        loading.value = false;
        controller = null;
      }
    }
  }

  async function open(id: string): Promise<void> {
    const request = ++detailRequestId;
    detailController?.abort();
    const current = new AbortController();
    detailController = current;
    detail.value = null;
    detailError.value = "";
    detailLoading.value = true;
    try {
      const response = await http.get<unknown>(`/api/review/records/${encodeURIComponent(id)}`, {
        signal: current.signal,
      });
      if (!isReviewRecord(response.data)) {
        throw new Error("审核记录的数据格式不符合约定，无法展示详情。");
      }
      if (request !== detailRequestId || current.signal.aborted) return;
      detail.value = response.data;
    } catch (cause) {
      if (request !== detailRequestId || current.signal.aborted) return;
      detailError.value = apiErrorMessage(cause, "审核记录") || "读取审核记录失败，请稍后重试。";
    } finally {
      if (request === detailRequestId) {
        detailLoading.value = false;
        detailController = null;
      }
    }
  }

  function clearDetail(): void {
    ++detailRequestId;
    detailController?.abort();
    detailController = null;
    detailLoading.value = false;
    detailError.value = "";
    detail.value = null;
  }

  onScopeDispose(() => {
    ++requestId;
    controller?.abort();
    clearDetail();
  });

  return {
    report: readonly(report),
    detail: readonly(detail),
    loading: readonly(loading),
    detailLoading: readonly(detailLoading),
    error: readonly(error),
    detailError: readonly(detailError),
    load,
    open,
    clearDetail,
  };
}
