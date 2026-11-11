package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"adm_jev/internal/creative/asset"
	"adm_jev/internal/creative/model"
	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
	"adm_jev/internal/material/service"
)

// ReviewService 编排素材抽取、下载、协议规则和 JEV 标签审核。
type ReviewService struct {
	Client  *jev.Client
	Fetcher *fetch.Fetcher
}

// Review 返回当前内容可观察的审核结果。下载或模型失败只写入 ReviewError，不丢弃已取得的结果。
func (s *ReviewService) Review(ctx context.Context, input string) (model.ReviewResult, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return model.ReviewResult{}, errors.New("内容为空")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	acquired, err := service.Acquire(ctx, input, s.Fetcher)
	if err != nil {
		return model.ReviewResult{}, err
	}
	bids := acquired.Bids
	if len(bids) == 0 {
		return model.ReviewResult{}, errors.New("没有可审核的 bid")
	}

	urls := make([]model.URLReport, 0)
	for _, report := range asset.Classify(bids) {
		urls = append(urls, urlReport(report))
	}
	verdict := asset.UnsecuredRequest(bids)
	unsecured := model.UnsecuredVerdict{
		Status:   verdict.Status,
		Evidence: make([]model.URLReport, 0, len(verdict.Evidence)),
		Notices:  make([]model.URLReport, 0, len(verdict.Notices)),
	}
	for _, report := range verdict.Evidence {
		unsecured.Evidence = append(unsecured.Evidence, urlReport(report))
	}
	for _, report := range verdict.Notices {
		unsecured.Notices = append(unsecured.Notices, urlReport(report))
	}

	files := acquired.Files
	client := s.Client
	if client == nil {
		client = &jev.Client{}
	}
	result := model.ReviewResult{
		Bids:        bids,
		URLs:        urls,
		Unsecured:   unsecured,
		Labels:      make([]model.LabelReview, 0),
		Model:       client.Model,
		ReviewError: "",
	}
	labels, reviewErr := asset.Submit(ctx, client, bids, files)
	for _, label := range labels {
		result.Labels = append(result.Labels, model.LabelReview{
			Label:       label.Label,
			SourceID:    label.SourceID,
			Status:      label.Status,
			Noul:        label.Noul,
			Probability: label.Probability,
		})
	}
	if reviewErr != nil {
		result.ReviewError = reviewErr.Error()
	}
	return result, nil
}

func urlReport(report asset.SchemeReport) model.URLReport {
	return model.URLReport{
		SourceID:  report.SourceID,
		URL:       report.URL,
		Scheme:    report.Scheme,
		Group:     report.Group,
		Unsecured: report.Unsecured,
	}
}
