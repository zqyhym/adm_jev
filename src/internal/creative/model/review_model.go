package model

import (
	"errors"
	"time"

	"adm_jev/internal/material/model"
)

const (
	ReviewStatusFailed  = "failed"
	ReviewStatusWarning = "warning"
	ReviewStatusSuccess = "success"
)

// ErrReviewNotFound 表示审核记录不存在或标识无法识别。
var ErrReviewNotFound = errors.New("审核记录不存在")

// ReviewResult 是一次素材审核的可观察结果。
type ReviewResult struct {
	ID          string            `json:"id,omitempty"`
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

// ReviewLabelStat 是写入记录、供报表汇总的一条标签。
type ReviewLabelStat struct {
	Label  string   `json:"label" bson:"label"`
	Status string   `json:"status" bson:"status"`
	Noul   *float64 `json:"noul" bson:"noul"`
}

// ReviewRecord 是一次 ADM 输入对应的审核记录。
type ReviewRecord struct {
	ID              string            `json:"id"`
	CreatedAt       time.Time         `json:"createdAt"`
	ElapsedMs       int64             `json:"elapsedMs"`
	Input           string            `json:"input,omitempty"`
	Status          string            `json:"status"`
	Error           string            `json:"error"`
	ReviewError     string            `json:"reviewError"`
	Model           string            `json:"model"`
	UnsecuredStatus string            `json:"unsecuredStatus"`
	Crids           []string          `json:"crids"`
	Adomains        []string          `json:"adomains"`
	BidCount        int               `json:"bidCount"`
	CreativeCount   int               `json:"creativeCount"`
	ScoredCount     int               `json:"scoredCount"`
	Labels          []ReviewLabelStat `json:"labels"`
	Result          *ReviewResult     `json:"result,omitempty"`
}

// ReviewReportQuery 是审核报表的筛选条件。To 是结束日期的次日零点，不包含在范围内。
type ReviewReportQuery struct {
	From     time.Time
	To       time.Time
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// ReviewLabelCount 是当前筛选下某个已评分标签的出现次数。
type ReviewLabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ReviewReport 是一页审核记录和当前筛选下的汇总。
type ReviewReport struct {
	Total    int                `json:"total"`
	Success  int                `json:"success"`
	Warning  int                `json:"warning"`
	Failed   int                `json:"failed"`
	Hit      int                `json:"hit"`
	Labels   []ReviewLabelCount `json:"labels"`
	Records  []ReviewRecord     `json:"records"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}
