<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const props = defineProps<{
  snapshotId: number;
  html: string;
  width: number;
  height: number;
}>();
const emit = defineEmits<{ blocked: [snapshotId: number, url: string] }>();
const frame = ref<HTMLIFrameElement | null>(null);
const sourceDocument = ref<string>();
const frameStyle = computed(() => ({
  width: `${Math.max(props.width, 1)}px`,
  height: `${Math.max(props.height, 1)}px`,
}));

function receiveBlocked(event: MessageEvent<unknown>): void {
  const currentWindow = frame.value?.contentWindow;
  if (!currentWindow || event.source !== currentWindow) return;

  const data = event.data;
  if (typeof data !== "object" || data === null || Array.isArray(data)) return;
  if (!("type" in data) || data.type !== "adm-blocked") return;
  if (!("url" in data) || typeof data.url !== "string" || !data.url.trim()) return;

  // A sandboxed srcdoc has an opaque origin; identify the actual frame instead.
  emit("blocked", props.snapshotId, data.url.trim());
}

onMounted(() => {
  // The iframe ref exists and the listener is active BEFORE srcdoc can execute.
  // The parent keys this component by snapshot id; tab changes only use v-show.
  window.addEventListener("message", receiveBlocked);
  sourceDocument.value = props.html;
});

onBeforeUnmount(() => {
  window.removeEventListener("message", receiveBlocked);
});
</script>

<template>
  <div class="preview-canvas" tabindex="0" role="region" aria-label="原始尺寸预览画布，可滚动查看">
    <div class="canvas-stage">
      <iframe
        ref="frame"
        title="广告创意预览"
        sandbox="allow-scripts"
        :srcdoc="sourceDocument"
        :style="frameStyle"
      ></iframe>
    </div>
  </div>
</template>
