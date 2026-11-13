// UNKNOWN 归因代码，与 src/internal/creative/asset/review.go 的 Reason 常量一一对应。
export const reasonLabels: Record<string, string> = {
  NOT_OBTAINED: "图片下载失败",
  NO_DATA: "无图片字节",
  IMAGE_SIZE: "尺寸超限",
  INVALID_IMAGE: "格式不支持或损坏",
  NO_SUBJECT: "无主体图片",
  HUB_FAILED: "模型请求失败",
  MISSING_ANSWER: "标签缺项或分数无效",
};

export function reasonText(reason?: string): string {
  if (!reason) return "—";
  return reasonLabels[reason] ?? reason;
}

// 一条记录里出现过的归因，按首次出现顺序去重。
export function recordReasons(labels: readonly { reason?: string }[]): string[] {
  const seen: string[] = [];
  for (const label of labels) {
    if (label.reason && !seen.includes(label.reason)) seen.push(label.reason);
  }
  return seen;
}