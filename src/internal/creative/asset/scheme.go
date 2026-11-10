package asset

import (
	"net/url"
	"strings"

	"adm_jev/internal/creative/model"
)

// SchemeReport 是一条 URL 的协议分组。通知地址即使使用 HTTP 也不进入 Unsecured。
type SchemeReport struct {
	SourceID  string
	URL       string
	Scheme    string
	Group     string
	Unsecured bool
}

// Classify 只检查来源清单里的地址，不扫描 ADM 原文。
func Classify(bids []model.BidSource) []SchemeReport {
	var reports []SchemeReport
	for _, bid := range bids {
		for _, asset := range bid.Assets {
			if asset.Kind != "image" && asset.Kind != "script" && asset.Kind != "iframe" && asset.Kind != "url" {
				continue
			}
			if asset.Role == "tracker" {
				continue
			}
			report := SchemeReport{SourceID: asset.SourceID, URL: asset.Value}
			if isNotice(asset.FieldPath) {
				report.Group = "notice"
				report.Scheme, _ = absoluteScheme(asset.Value)
				reports = append(reports, report)
				continue
			}
			if asset.Role != "creative" {
				continue
			}
			scheme, unresolved := absoluteScheme(asset.Value)
			if unresolved {
				report.Group = "unresolved"
				reports = append(reports, report)
				continue
			}
			report.Group = "creative"
			report.Scheme = scheme
			report.Unsecured = scheme == "http"
			reports = append(reports, report)
		}
	}
	return reports
}

func isNotice(fieldPath string) bool {
	return strings.HasSuffix(fieldPath, ".nurl") || strings.HasSuffix(fieldPath, ".burl") || strings.HasSuffix(fieldPath, ".lurl")
}

func absoluteScheme(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" || strings.Contains(raw, "${") {
		return "", true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", true
	}
	return strings.ToLower(parsed.Scheme), false
}
