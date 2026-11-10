package model

const (
	ParseOK      = "OK"
	ParsePartial = "PARTIAL"
	ParseFailed  = "FAILED"
)

// Asset 是一条可追溯的素材来源。Obtained 只在创意图片下载并校验成功后为 true。
type Asset struct {
	SourceID    string `json:"sourceId"`
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	FieldPath   string `json:"fieldPath"`
	Role        string `json:"role"`
	Obtained    bool   `json:"obtained"`
	ContentHash string `json:"contentHash,omitempty"`
	FetchError  string `json:"fetchError,omitempty"`
}

// BidSource 是一个 bid 的 Banner 素材清单。ImpSecure 为空表示没有关联到请求。
type BidSource struct {
	BidPath     string   `json:"bidPath"`
	Crid        string   `json:"crid"`
	Adomain     string   `json:"adomain"`
	W           int      `json:"w"`
	H           int      `json:"h"`
	ImpID       string   `json:"impid"`
	ImpSecure   *bool    `json:"impSecure,omitempty"`
	AdmHash     string   `json:"admHash"`
	AdmTypes    []string `json:"admTypes"`
	ParseStatus string   `json:"parseStatus"`
	Failure     string   `json:"failure"`
	Assets      []Asset  `json:"assets"`
}
