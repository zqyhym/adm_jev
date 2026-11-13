package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"adm_jev/internal/creative/model"
)

// TestReviewRepositoryRoundTrip 验证审核记录写入后可按状态和时间筛选，且列表不返回原文和完整结果。
func TestReviewRepositoryRoundTrip(t *testing.T) {
	uri := os.Getenv("ADM_JEV_MONGO_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer connectCancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skip(err)
	}
	if err = client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skip(err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = client.Disconnect(closeCtx)
	}()

	database := client.Database("adm_jev_review_test")
	collection := database.Collection(reviewCollection)
	dropCtx, dropCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = collection.Drop(dropCtx)
	dropCancel()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = collection.Drop(cleanupCtx)
	})

	repo := NewReviewRepository(database)
	indexCtx, indexCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = repo.EnsureIndexes(indexCtx)
	indexCancel()
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	noul := 0.8
	successResult := &model.ReviewResult{
		Unsecured: model.UnsecuredVerdict{Status: "HIT"},
		Model:     "fake-jev",
	}
	successID, err := repo.Save(context.Background(), model.ReviewRecord{
		CreatedAt:       base,
		ElapsedMs:       1200,
		Input:           "success-input",
		Status:          model.ReviewStatusSuccess,
		Model:           "fake-jev",
		UnsecuredStatus: "HIT",
		Crids:           []string{"cr-1"},
		Adomains:        []string{"example.com"},
		BidCount:        1,
		CreativeCount:   1,
		ScoredCount:     1,
		Labels:          []model.ReviewLabelStat{{Label: "Nudity", Status: "SCORED", Noul: &noul}, {Label: "Gambling", Status: "UNKNOWN", Reason: "IMAGE_SIZE"}, {Label: "Violence", Status: "UNKNOWN"}},
		Result:          successResult,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(context.Background(), model.ReviewRecord{
		CreatedAt:       base.Add(time.Minute),
		ElapsedMs:       800,
		Input:           "warning-input",
		Status:          model.ReviewStatusWarning,
		ReviewError:     "模型超时",
		Model:           "fake-jev",
		UnsecuredStatus: "HIT",
		Crids:           []string{"cr-2"},
		Adomains:        []string{"other.example"},
		Labels:          []model.ReviewLabelStat{{Label: "Gambling", Status: "SCORED", Noul: &noul}, {Label: "Dating", Status: "UNKNOWN", Reason: "MISSING_ANSWER"}},
		Result:          &model.ReviewResult{Model: "fake-jev", ReviewError: "模型超时"},
	}); err != nil {
		t.Fatal(err)
	}
	failedID, err := repo.Save(context.Background(), model.ReviewRecord{
		CreatedAt: base,
		ElapsedMs: 5,
		Input:     "  ",
		Status:    model.ReviewStatusFailed,
		Error:     "内容为空",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Save(context.Background(), model.ReviewRecord{
		CreatedAt: base.Add(-30 * 24 * time.Hour),
		Input:     "old-input",
		Status:    model.ReviewStatusFailed,
		Error:     "内容为空",
	}); err != nil {
		t.Fatal(err)
	}

	window := model.ReviewReportQuery{
		From:     base.Add(-time.Hour),
		To:       base.Add(2 * time.Hour),
		Page:     1,
		PageSize: 20,
	}
	report, err := repo.List(context.Background(), window)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 3 || report.Success != 1 || report.Warning != 1 || report.Failed != 1 || report.Hit != 2 {
		t.Fatalf("report=%+v", report)
	}
	if len(report.Records) != 3 || report.Records[0].Input != "" || report.Records[0].Result != nil {
		t.Fatalf("records=%+v", report.Records)
	}
	if report.Records[0].Status != model.ReviewStatusWarning {
		t.Fatalf("first=%s", report.Records[0].Status)
	}
	if len(report.Labels) != 2 || report.Labels[0].Label != "Gambling" || report.Labels[0].Count != 1 || report.Labels[1].Label != "Nudity" || report.Labels[1].Count != 1 {
		t.Fatalf("labels=%+v", report.Labels)
	}
	// 无 Reason 的历史 UNKNOWN 行不进入归因汇总。
	if len(report.Reasons) != 2 || report.Reasons[0].Reason != "IMAGE_SIZE" || report.Reasons[0].Count != 1 || report.Reasons[1].Reason != "MISSING_ANSWER" || report.Reasons[1].Count != 1 {
		t.Fatalf("reasons=%+v", report.Reasons)
	}

	filtered := window
	filtered.Status = model.ReviewStatusSuccess
	filtered.Keyword = "cr-1"
	report, err = repo.List(context.Background(), filtered)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 1 || len(report.Records) != 1 || report.Records[0].ID != successID || report.Records[0].Input != "" || report.Records[0].Result != nil {
		t.Fatalf("filtered=%+v", report)
	}

	detail, err := repo.Get(context.Background(), successID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Input != "success-input" || detail.Result == nil || detail.Result.Model != "fake-jev" || detail.Labels[1].Reason != "IMAGE_SIZE" {
		t.Fatalf("detail=%+v", detail)
	}
	failed, err := repo.Get(context.Background(), failedID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Input != "  " || failed.Result != nil || failed.Error != "内容为空" {
		t.Fatalf("failed=%+v", failed)
	}
	if _, err = repo.Get(context.Background(), "not-an-id"); err != model.ErrReviewNotFound {
		t.Fatalf("err=%v", err)
	}
}
