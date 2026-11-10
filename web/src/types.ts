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

export interface ReviewAsset {
  sourceId: string;
  kind: string;
  value: string;
  fieldPath: string;
  role: string;
  obtained: boolean;
  contentHash?: string;
  fetchError?: string;
}

export interface ReviewBid {
  bidPath: string;
  crid: string;
  adomain: string;
  w: number;
  h: number;
  impid: string;
  impSecure?: boolean;
  admHash: string;
  admTypes: string[];
  parseStatus: string;
  failure: string;
  assets: ReviewAsset[];
}

export interface UrlReport {
  sourceId: string;
  url: string;
  scheme: string;
  group: string;
  unsecured: boolean;
}

export interface UnsecuredVerdict {
  status: string;
  evidence: UrlReport[];
  notices: UrlReport[];
}

export interface LabelReview {
  label: string;
  sourceId: string;
  status: string;
  noul: number | null;
  probability: number | null;
}

export interface ReviewResult {
  bids: ReviewBid[];
  urls: UrlReport[];
  unsecured: UnsecuredVerdict;
  labels: LabelReview[];
  model: string;
  reviewError: string;
}

export interface ReviewSnapshot {
  id: number;
  input: string;
  data: ReviewResult;
  elapsedMs: number;
}
