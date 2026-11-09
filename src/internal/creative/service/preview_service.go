package service

import (
	"adm_jev/internal/creative/model"
	"adm_jev/internal/creative/preview"
)

type PreviewService struct{}

func (s *PreviewService) Preview(input string) (model.Result, error) {
	return preview.Parse(input)
}
