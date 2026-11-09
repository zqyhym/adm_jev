export interface ImageAsset {
  url: string;
  source: string;
  tracker: boolean;
}

export interface PreviewResult {
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

// A successful response and its blocked URLs are published as one snapshot.
export interface PreviewSnapshot {
  id: number;
  input: string;
  data: PreviewResult;
  blocked: string[];
}
