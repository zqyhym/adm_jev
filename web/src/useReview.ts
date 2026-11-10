import { onScopeDispose, readonly, ref, shallowRef } from "vue";
import { apiErrorMessage, http } from "./api/http";
import type {
  LabelReview,
  ReviewAsset,
  ReviewBid,
  ReviewResult,
  ReviewSnapshot,
  UnsecuredVerdict,
  UrlReport,
} from "./types";

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isNullableNumber(value: unknown): value is number | null {
  return value === null || (typeof value === "number" && Number.isFinite(value));
}

function isStringList(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function isReviewAsset(value: unknown): value is ReviewAsset {
  if (!isObject(value)) return false;
  if (!isString(value.sourceId) || !isString(value.kind) || !isString(value.value) || !isString(value.fieldPath) || !isString(value.role) || typeof value.obtained !== "boolean") {
    return false;
  }
  if (value.contentHash !== undefined && !isString(value.contentHash)) return false;
  if (value.fetchError !== undefined && !isString(value.fetchError)) return false;
  return true;
}

function isReviewBid(value: unknown): value is ReviewBid {
  if (!isObject(value)) return false;
  if (!isString(value.bidPath) || !isString(value.crid) || !isString(value.adomain) || !isString(value.impid) || !isString(value.admHash) || !isString(value.parseStatus) || !isString(value.failure)) {
    return false;
  }
  if (typeof value.w !== "number" || !Number.isFinite(value.w) || typeof value.h !== "number" || !Number.isFinite(value.h)) return false;
  if (value.impSecure !== undefined && typeof value.impSecure !== "boolean") return false;
  if (!isStringList(value.admTypes)) return false;
  return Array.isArray(value.assets) && value.assets.every(isReviewAsset);
}

function isUrlReport(value: unknown): value is UrlReport {
  if (!isObject(value)) return false;
  return isString(value.sourceId)
    && isString(value.url)
    && isString(value.scheme)
    && isString(value.group)
    && typeof value.unsecured === "boolean";
}

function isUnsecuredVerdict(value: unknown): value is UnsecuredVerdict {
  if (!isObject(value) || !isString(value.status)) return false;
  return Array.isArray(value.evidence)
    && value.evidence.every(isUrlReport)
    && Array.isArray(value.notices)
    && value.notices.every(isUrlReport);
}

function isLabelReview(value: unknown): value is LabelReview {
  if (!isObject(value)) return false;
  return isString(value.label)
    && isString(value.sourceId)
    && isString(value.status)
    && isNullableNumber(value.noul)
    && isNullableNumber(value.probability);
}

function isReviewResult(value: unknown): value is ReviewResult {
  if (!isObject(value)) return false;
  return Array.isArray(value.bids)
    && value.bids.every(isReviewBid)
    && Array.isArray(value.urls)
    && value.urls.every(isUrlReport)
    && isUnsecuredVerdict(value.unsecured)
    && Array.isArray(value.labels)
    && value.labels.every(isLabelReview)
    && isString(value.model)
    && isString(value.reviewError);
}

export function useReview() {
  const snapshot = shallowRef<ReviewSnapshot | null>(null);
  const loading = ref(false);
  const error = ref("");
  const submittedInput = ref<string | null>(null);
  let requestId = 0;
  let controller: AbortController | null = null;

  async function run(input: string): Promise<boolean> {
    const id = ++requestId;
    controller?.abort();
    controller = null;
    error.value = "";
    submittedInput.value = input;

    if (!input.trim()) {
      loading.value = false;
      error.value = "请先粘贴 Bid Response 或 ADM 内容。";
      return false;
    }

    const currentController = new AbortController();
    controller = currentController;
    loading.value = true;
    const startedAt = performance.now();

    try {
      const response = await http.post<unknown>("/api/review", { input }, {
        signal: currentController.signal,
        timeout: 330_000,
      });
      if (!isReviewResult(response.data)) {
        throw new Error("审核服务返回的数据格式不符合约定，无法展示结果。");
      }
      if (id !== requestId || currentController.signal.aborted) return false;

      snapshot.value = {
        id,
        input,
        data: response.data,
        elapsedMs: Math.max(0, Math.round(performance.now() - startedAt)),
      };
      return true;
    } catch (cause) {
      if (id !== requestId || currentController.signal.aborted) return false;
      error.value = apiErrorMessage(cause, "审核服务") || "审核失败，请稍后重试。";
      return false;
    } finally {
      if (id === requestId) {
        loading.value = false;
        controller = null;
      }
    }
  }

  function clear(): void {
    ++requestId;
    controller?.abort();
    controller = null;
    loading.value = false;
    error.value = "";
    submittedInput.value = null;
    snapshot.value = null;
  }

  onScopeDispose(clear);

  return {
    snapshot: readonly(snapshot),
    loading: readonly(loading),
    error: readonly(error),
    submittedInput: readonly(submittedInput),
    run,
    clear,
  };
}
