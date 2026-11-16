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
  reason?: string;
  upstreamModel?: string;
  noul: number | null;
  probability: number | null;
}

export interface ReviewResult {
  id?: string;
  bids: ReviewBid[];
  urls: UrlReport[];
  unsecured: UnsecuredVerdict;
  labels: LabelReview[];
  model: string;
  upstreamModel?: string;
  reviewError: string;
}

export interface ReviewSnapshot {
  id: number;
  input: string;
  data: ReviewResult;
  elapsedMs: number;
}

export interface ReviewLabelStat {
  label: string;
  status: string;
  reason?: string;
  upstreamModel?: string;
  noul: number | null;
}

export interface ReviewRecord {
  id: string;
  createdAt: string;
  elapsedMs: number;
  input?: string;
  status: string;
  error: string;
  reviewError: string;
  model: string;
  upstreamModel?: string;
  unsecuredStatus: string;
  crids: string[];
  adomains: string[];
  bidCount: number;
  creativeCount: number;
  scoredCount: number;
  labels: ReviewLabelStat[];
  result?: ReviewResult | null;
}

export interface ReviewLabelCount {
  label: string;
  count: number;
}

export interface ReviewReasonCount {
  reason: string;
  count: number;
}

export interface ReviewReport {
  total: number;
  success: number;
  warning: number;
  failed: number;
  hit: number;
  labels: ReviewLabelCount[];
  reasons: ReviewReasonCount[];
  records: ReviewRecord[];
  page: number;
  pageSize: number;
}

export interface ReviewReportQuery {
  from: string;
  to: string;
  status: string;
  keyword: string;
  page: number;
  pageSize: number;
}
