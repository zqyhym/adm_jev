import "./style.css";
import { sampleInput } from "./sample";

interface ImageAsset {
  url: string;
  source: string;
  tracker: boolean;
}

interface PreviewResult {
  format: string;
  width: number;
  height: number;
  crid: string;
  adomain: string;
  images: ImageAsset[] | null;
  texts: string[] | null;
  blocked: string[] | null;
  previewHtml: string;
}

const sourceName: Record<string, string> = {
  tag: "标签",
  srcset: "srcset",
  background: "背景",
  token: "令牌",
  native: "原生",
  media: "媒体文件",
};

const formatName: Record<string, string> = {
  banner: "横幅",
  native: "原生",
  vast: "视频",
};

const input = document.querySelector<HTMLTextAreaElement>("#input")!;
const error = document.querySelector<HTMLParagraphElement>("#error")!;
const result = document.querySelector<HTMLElement>("#result")!;
const meta = document.querySelector<HTMLDListElement>("#meta")!;
const images = document.querySelector<HTMLDivElement>("#images")!;
const texts = document.querySelector<HTMLUListElement>("#texts")!;
const blocked = document.querySelector<HTMLUListElement>("#blocked")!;
const frame = document.querySelector<HTMLIFrameElement>("#frame")!;

const seenBlocked = new Set<string>();

document.querySelector("#fill")!.addEventListener("click", () => {
  input.value = sampleInput;
});

document.querySelector("#run")!.addEventListener("click", () => {
  void preview();
});

document.querySelector("#clear")!.addEventListener("click", () => {
  input.value = "";
  input.focus();
  error.hidden = true;
  error.textContent = "";
  result.hidden = true;
  seenBlocked.clear();
  meta.replaceChildren();
  images.replaceChildren();
  texts.replaceChildren();
  blocked.replaceChildren();
  frame.srcdoc = "";
  frame.removeAttribute("style");
});

window.addEventListener("message", (event) => {
  const data = event.data as { type?: string; url?: string } | null;
  if (!data || data.type !== "adm-blocked" || !data.url) {
    return;
  }
  addBlocked(data.url);
});

async function preview(): Promise<void> {
  error.hidden = true;
  const response = await fetch("/api/preview", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ input: input.value }),
  });
  const body = (await response.json()) as PreviewResult & { error?: string };
  if (!response.ok) {
    result.hidden = true;
    error.hidden = false;
    error.textContent = body.error || "预览失败";
    return;
  }
  render(body);
}

function render(data: PreviewResult): void {
  result.hidden = false;
  seenBlocked.clear();
  meta.replaceChildren();
  addMeta("格式", formatName[data.format] || data.format);
  addMeta("创意", data.crid || "—");
  addMeta("广告主", data.adomain || "—");
  addMeta("尺寸", `${data.width} × ${data.height}`);

  images.replaceChildren();
  const creatives = (data.images || []).filter((item) => !item.tracker);
  if (creatives.length === 0) {
    images.textContent = "没有抽出可展示的素材。";
  }
  for (const item of creatives) {
    const figure = document.createElement("figure");
    const img = document.createElement("img");
    img.src = item.url;
    img.alt = sourceName[item.source] || item.source;
    const caption = document.createElement("figcaption");
    caption.textContent = `${sourceName[item.source] || item.source}：${item.url}`;
    figure.append(img, caption);
    images.append(figure);
  }

  fillList(texts, data.texts || [], "没有可见文字。");
  blocked.replaceChildren();
  for (const url of data.blocked || []) {
    addBlocked(url);
  }
  if ((data.blocked || []).length === 0) {
    fillList(blocked, [], "没有拦截地址。");
  }

  frame.style.width = `${Math.max(data.width, 1)}px`;
  frame.style.height = `${Math.max(data.height, 1)}px`;
  frame.srcdoc = data.previewHtml;
}

function addMeta(name: string, value: string): void {
  const dt = document.createElement("dt");
  const dd = document.createElement("dd");
  dt.textContent = name;
  dd.textContent = value;
  meta.append(dt, dd);
}

function fillList(list: HTMLUListElement, values: string[], empty: string): void {
  list.replaceChildren();
  if (values.length === 0) {
    const item = document.createElement("li");
    item.textContent = empty;
    list.append(item);
    return;
  }
  for (const value of values) {
    const item = document.createElement("li");
    item.textContent = value;
    list.append(item);
  }
}

function addBlocked(url: string): void {
  if (seenBlocked.has(url)) {
    return;
  }
  if (seenBlocked.size === 0) {
    blocked.replaceChildren();
  }
  seenBlocked.add(url);
  const item = document.createElement("li");
  item.textContent = url;
  blocked.append(item);
}
