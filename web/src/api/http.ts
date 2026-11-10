import axios from "axios";

export const http = axios.create({ timeout: 30_000 });

export function apiErrorMessage(error: unknown, service: string): string {
  if (axios.isCancel(error) || (axios.isAxiosError(error) && error.code === "ERR_CANCELED")) {
    return "";
  }
  if (axios.isAxiosError(error)) {
    const body = error.response?.data;
    if (typeof body === "object" && body !== null && !Array.isArray(body)) {
      const message = (body as Record<string, unknown>).error;
      if (typeof message === "string" && message.trim()) return message.trim();
    }
    if (!error.response) {
      return `无法连接${service}，请检查网络或服务状态后重试。`;
    }
    return `${service}失败（HTTP ${error.response.status}），请稍后重试。`;
  }
  return error instanceof Error ? error.message : `${service}失败，请稍后重试。`;
}
