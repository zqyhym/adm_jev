package service

import (
	"context"

	"adm_jev/internal/material/fetch"
	"adm_jev/internal/material/model"
	"adm_jev/internal/material/parser"
)

// Result 是一次素材获取的清单和已下载字节。
type Result struct {
	Bids  []model.BidSource
	Files map[string][]byte
}

// Acquire 解析 Bid Response 或裸 ADM，并下载 Banner 中的创意图片。
func Acquire(ctx context.Context, input string, fetcher *fetch.Fetcher) (Result, error) {
	bids, err := parser.Parse(input)
	if err != nil {
		return Result{}, err
	}
	return Result{Bids: bids, Files: fetch.Fetch(ctx, bids, fetcher)}, nil
}
