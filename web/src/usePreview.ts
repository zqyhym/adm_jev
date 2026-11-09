import { onScopeDispose, readonly, ref, shallowRef } from "vue";
import type { ImageAsset, PreviewResult, PreviewSnapshot } from "./types";

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStringList(value: unknown): value is string[] | null {
  return value === null || (Array.isArray(value) && value.every((item) => typeof item === "string"));
}

function isImageAsset(value: unknown): value is ImageAsset {
  return isObject(value)
    && typeof value.url === "string"
    && typeof value.source === "string"
    && typeof value.tracker === "boolean";
}

function isPreviewResult(value: unknown): value is PreviewResult {
  return isObject(value)
    && typeof value.format === "string"
    && typeof value.width === "number" && Number.isFinite(value.width)
    && typeof value.height === "number" && Number.isFinite(value.height)
    && typeof value.crid === "string"
    && typeof value.adomain === "string"
    && typeof value.previewHtml === "string"
    && (value.images === null || (Array.isArray(value.images) && value.images.every(isImageAsset)))
    && isStringList(value.texts)
    && isStringList(value.blocked);
}

export function usePreview() {
  const snapshot = shallowRef<PreviewSnapshot | null>(null);
  const loading = ref(false);
  const error = ref("");
  const submittedInput = ref<string | null>(null);
  let requestId = 0;
  let controller: AbortController | null = null;
  let seenBlocked = new Set<string>();

  async function run(input: string): Promise<boolean> {
    // Invalidate first: even an abort rejection/finally belongs to the old request.
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

    try {
      const response = await fetch("/api/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input }),
        signal: currentController.signal,
      });

      let body: unknown;
      try {
        body = await response.json();
      } catch {
        throw new Error(`预览服务未返回有效 JSON（HTTP ${response.status}），请稍后重试。`);
      }

      if (!response.ok) {
        const message = isObject(body) && typeof body.error === "string" ? body.error.trim() : "";
        throw new Error(message || `预览失败（HTTP ${response.status}），请检查输入或稍后重试。`);
      }
      if (!isPreviewResult(body)) {
        throw new Error("预览服务返回的数据格式不符合约定，无法展示结果。");
      }
      if (id !== requestId || currentController.signal.aborted) return false;

      seenBlocked = new Set((body.blocked ?? []).map((url) => url.trim()).filter(Boolean));
      // No watcher clears child-emitted messages after the new iframe has mounted.
      snapshot.value = { id, input, data: body, blocked: [...seenBlocked] };
      return true;
    } catch (cause) {
      if (id !== requestId || currentController.signal.aborted) return false;
      error.value = cause instanceof TypeError
        ? "无法连接预览服务，请检查网络或服务状态后重试。"
        : cause instanceof Error ? cause.message : "预览失败，请稍后重试。";
      return false;
    } finally {
      if (id === requestId) {
        loading.value = false;
        controller = null;
      }
    }
  }

  function addBlocked(snapshotId: number, url: string): void {
    const current = snapshot.value;
    const normalized = url.trim();
    // A previous iframe can still have queued messages when a new result arrives.
    if (!current || current.id !== snapshotId || !normalized || seenBlocked.has(normalized)) return;
    seenBlocked.add(normalized);
    snapshot.value = { ...current, blocked: [...seenBlocked] };
  }

  function clear(): void {
    ++requestId;
    controller?.abort();
    controller = null;
    loading.value = false;
    error.value = "";
    submittedInput.value = null;
    snapshot.value = null;
    seenBlocked.clear();
  }

  onScopeDispose(clear);

  return {
    snapshot: readonly(snapshot),
    loading: readonly(loading),
    error: readonly(error),
    submittedInput: readonly(submittedInput),
    run,
    addBlocked,
    clear,
  };
}
