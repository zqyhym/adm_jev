package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"adm_jev/internal/creative/asset"
	"adm_jev/internal/creative/model"
	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
	"adm_jev/internal/material/service"
)

// ReviewStore 保存并查询一次 ADM 输入对应的审核记录。
type ReviewStore interface {
	Save(ctx context.Context, record model.ReviewRecord) (string, error)
	List(ctx context.Context, query model.ReviewReportQuery) (model.ReviewReport, error)
	Get(ctx context.Context, id string) (model.ReviewRecord, error)
}

// ReviewService 编排素材抽取、下载、协议规则和 JEV 标签审核。
type ReviewService struct {
	Client  *jev.Client
	Fetcher *fetch.Fetcher
	Records ReviewStore
}

// Review 返回当前内容可观察的审核结果。下载或模型失败只写入 ReviewError，不丢弃已取得的结果。
// 只要收到 ADM 输入，无论成败都尝试写入一条审核记录。
func (s *ReviewService) Review(ctx context.Context, input string) (result model.ReviewResult, err error) {
	started := time.Now()
	rawInput := input
	defer func() {
		if s.Records == nil {
			return
		}
		record := model.ReviewRecord{
			CreatedAt: time.Now(),
			ElapsedMs: time.Since(started).Milliseconds(),
			Input:     rawInput,
			Crids:     []string{},
			Adomains:  []string{},
			Labels:    []model.ReviewLabelStat{},
		}
		if err != nil {
			record.Status = model.ReviewStatusFailed
			record.Error = err.Error()
		} else {
			record.Status = model.ReviewStatusSuccess
			if result.ReviewError != "" {
				record.Status = model.ReviewStatusWarning
			}
			record.ReviewError = result.ReviewError
			record.Model = result.Model
			record.UnsecuredStatus = result.Unsecured.Status
			record.BidCount = len(result.Bids)
			for _, bid := range result.Bids {
				if bid.Crid != "" {
					record.Crids = append(record.Crids, bid.Crid)
				}
				if bid.Adomain != "" {
					record.Adomains = append(record.Adomains, bid.Adomain)
				}
				for _, item := range bid.Assets {
					if item.Kind == "image" && item.Role == "creative" {
						record.CreativeCount++
					}
				}
			}
			for _, label := range result.Labels {
				if label.SourceID == "" {
					continue
				}
				record.Labels = append(record.Labels, model.ReviewLabelStat{
					Label:  label.Label,
					Status: label.Status,
					Noul:   label.Noul,
				})
				if label.Status == asset.StatusScored {
					record.ScoredCount++
				}
			}
			stored := result
			stored.ID = ""
			record.Result = &stored
		}
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		id, saveErr := s.Records.Save(saveCtx, record)
		if saveErr != nil {
			log.Printf("审核记录保存失败: %v", saveErr)
			return
		}
		result.ID = id
	}()

	input = strings.TrimSpace(input)
	if input == "" {
		err = errors.New("内容为空")
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	acquired, err := service.Acquire(ctx, input, s.Fetcher)
	if err != nil {
		return result, err
	}
	bids := acquired.Bids
	if len(bids) == 0 {
		err = errors.New("没有可审核的 bid")
		return result, err
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
	result = model.ReviewResult{
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

// List 返回当前筛选下的审核报表。
func (s *ReviewService) List(ctx context.Context, query model.ReviewReportQuery) (model.ReviewReport, error) {
	if s.Records == nil {
		return model.ReviewReport{}, errors.New("审核记录未启用")
	}
	return s.Records.List(ctx, query)
}

// Get 返回一条审核记录。
func (s *ReviewService) Get(ctx context.Context, id string) (model.ReviewRecord, error) {
	if s.Records == nil {
		return model.ReviewRecord{}, errors.New("审核记录未启用")
	}
	return s.Records.Get(ctx, id)
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
