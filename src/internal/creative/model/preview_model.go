package model

type Image struct {
	URL     string `json:"url"`
	Source  string `json:"source"`
	Tracker bool   `json:"tracker"`
}

type Result struct {
	Format      string   `json:"format"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	Crid        string   `json:"crid"`
	Adomain     string   `json:"adomain"`
	Images      []Image  `json:"images"`
	Texts       []string `json:"texts"`
	Blocked     []string `json:"blocked"`
	PreviewHTML string   `json:"previewHtml"`
}
