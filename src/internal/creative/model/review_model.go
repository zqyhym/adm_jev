package model

import "adm_jev/internal/material/model"

// ReviewResult 是一次素材审核的可观察结果。
type ReviewResult struct {
	Bids        []model.BidSource `json:"bids"`
	URLs        []URLReport       `json:"urls"`
	Unsecured   UnsecuredVerdict  `json:"unsecured"`
	Labels      []LabelReview     `json:"labels"`
	Model       string            `json:"model"`
	ReviewError string            `json:"reviewError"`
}

// URLReport 是一条来源地址的协议分组。
type URLReport struct {
	SourceID  string `json:"sourceId"`
	URL       string `json:"url"`
	Scheme    string `json:"scheme"`
	Group     string `json:"group"`
	Unsecured bool   `json:"unsecured"`
}

// UnsecuredVerdict 是 Unsecured Request 的规则结论。
type UnsecuredVerdict struct {
	Status   string      `json:"status"`
	Evidence []URLReport `json:"evidence"`
	Notices  []URLReport `json:"notices"`
}

// LabelReview 是一张图片上的一个视觉标签分数。
type LabelReview struct {
	Label       string   `json:"label"`
	SourceID    string   `json:"sourceId"`
	Status      string   `json:"status"`
	Noul        *float64 `json:"noul"`
	Probability *float64 `json:"probability"`
}
