package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"adm_jev/internal/creative/model"
)

const reviewCollection = "adm_review_records"

// ReviewRepository 把审核记录写入 Mongo 集合 adm_review_records。
type ReviewRepository struct {
	collection *mongo.Collection
}

func NewReviewRepository(database *mongo.Database) *ReviewRepository {
	return &ReviewRepository{collection: database.Collection(reviewCollection)}
}

// EnsureIndexes 创建报表查询用的索引。
func (r *ReviewRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "createdAt", Value: -1}}},
		{Keys: bson.D{{Key: "unsecuredStatus", Value: 1}, {Key: "createdAt", Value: -1}}},
	})
	if err != nil {
		return fmt.Errorf("创建审核记录索引失败: %w", err)
	}
	return nil
}

// Save 写入一条审核记录并返回其标识。
func (r *ReviewRepository) Save(ctx context.Context, record model.ReviewRecord) (string, error) {
	if record.Crids == nil {
		record.Crids = []string{}
	}
	if record.Adomains == nil {
		record.Adomains = []string{}
	}
	if record.Labels == nil {
		record.Labels = []model.ReviewLabelStat{}
	}
	inserted, err := r.collection.InsertOne(ctx, reviewDocument{
		CreatedAt:       record.CreatedAt,
		ElapsedMs:       record.ElapsedMs,
		Input:           record.Input,
		Status:          record.Status,
		Error:           record.Error,
		ReviewError:     record.ReviewError,
		Model:           record.Model,
		UnsecuredStatus: record.UnsecuredStatus,
		Crids:           record.Crids,
		Adomains:        record.Adomains,
		BidCount:        record.BidCount,
		CreativeCount:   record.CreativeCount,
		ScoredCount:     record.ScoredCount,
		Labels:          record.Labels,
		Result:          record.Result,
	})
	if err != nil {
		return "", fmt.Errorf("写入审核记录失败: %w", err)
	}
	id, ok := inserted.InsertedID.(bson.ObjectID)
	if !ok {
		return "", errors.New("审核记录缺少标识")
	}
	return id.Hex(), nil
}

// List 按筛选条件返回汇总和当前页。列表不包含原文和完整审核结果。
func (r *ReviewRepository) List(ctx context.Context, query model.ReviewReportQuery) (model.ReviewReport, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	filter := bson.D{{Key: "createdAt", Value: bson.D{
		{Key: "$gte", Value: query.From},
		{Key: "$lt", Value: query.To},
	}}}
	if query.Status != "" {
		filter = append(filter, bson.E{Key: "status", Value: query.Status})
	}
	if query.Keyword != "" {
		pattern := regexp.QuoteMeta(query.Keyword)
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "crids", Value: bson.D{{Key: "$regex", Value: pattern}}}},
			bson.D{{Key: "adomains", Value: bson.D{{Key: "$regex", Value: pattern}}}},
		}})
	}
	skip := int64(page-1) * int64(pageSize)
	cursor, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$facet", Value: bson.D{
			{Key: "records", Value: bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}}},
				bson.D{{Key: "$skip", Value: skip}},
				bson.D{{Key: "$limit", Value: pageSize}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "input", Value: 0}, {Key: "result", Value: 0}}}},
			}},
			{Key: "total", Value: bson.A{bson.D{{Key: "$count", Value: "count"}}}},
			{Key: "success", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: model.ReviewStatusSuccess}}}},
				bson.D{{Key: "$count", Value: "count"}},
			}},
			{Key: "warning", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: model.ReviewStatusWarning}}}},
				bson.D{{Key: "$count", Value: "count"}},
			}},
			{Key: "failed", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: model.ReviewStatusFailed}}}},
				bson.D{{Key: "$count", Value: "count"}},
			}},
			{Key: "hit", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "unsecuredStatus", Value: "HIT"}}}},
				bson.D{{Key: "$count", Value: "count"}},
			}},
			{Key: "labels", Value: bson.A{
				bson.D{{Key: "$unwind", Value: "$labels"}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "labels.status", Value: "SCORED"}}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$labels.label"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
			}},
			{Key: "reasons", Value: bson.A{
				bson.D{{Key: "$unwind", Value: "$labels"}},
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "labels.status", Value: "UNKNOWN"},
					{Key: "labels.reason", Value: bson.D{{Key: "$nin", Value: bson.A{"", nil}}}},
				}}},
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$labels.reason"},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
			}},
		}}},
	})
	report := model.ReviewReport{
		Labels:   []model.ReviewLabelCount{},
		Reasons:  []model.ReviewReasonCount{},
		Records:  []model.ReviewRecord{},
		Page:     page,
		PageSize: pageSize,
	}
	if err != nil {
		return report, fmt.Errorf("查询审核记录失败: %w", err)
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
		if err = cursor.Err(); err != nil {
			return report, fmt.Errorf("查询审核记录失败: %w", err)
		}
		return report, nil
	}
	var facet reviewFacet
	if err = cursor.Decode(&facet); err != nil {
		return report, fmt.Errorf("查询审核记录失败: %w", err)
	}
	if len(facet.Total) > 0 {
		report.Total = facet.Total[0].Count
	}
	if len(facet.Success) > 0 {
		report.Success = facet.Success[0].Count
	}
	if len(facet.Warning) > 0 {
		report.Warning = facet.Warning[0].Count
	}
	if len(facet.Failed) > 0 {
		report.Failed = facet.Failed[0].Count
	}
	if len(facet.Hit) > 0 {
		report.Hit = facet.Hit[0].Count
	}
	for _, label := range facet.Labels {
		report.Labels = append(report.Labels, model.ReviewLabelCount{Label: label.Label, Count: label.Count})
	}
	for _, reason := range facet.Reasons {
		report.Reasons = append(report.Reasons, model.ReviewReasonCount{Reason: reason.Reason, Count: reason.Count})
	}
	for _, doc := range facet.Records {
		report.Records = append(report.Records, documentRecord(doc, false))
	}
	return report, nil
}

// Get 读取一条审核记录，包含原文和完整结果。
func (r *ReviewRepository) Get(ctx context.Context, id string) (model.ReviewRecord, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return model.ReviewRecord{}, model.ErrReviewNotFound
	}
	var doc reviewDocument
	err = r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: objectID}}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.ReviewRecord{}, model.ErrReviewNotFound
	}
	if err != nil {
		return model.ReviewRecord{}, fmt.Errorf("读取审核记录失败: %w", err)
	}
	return documentRecord(doc, true), nil
}

type reviewDocument struct {
	ID              bson.ObjectID           `bson:"_id,omitempty"`
	CreatedAt       time.Time               `bson:"createdAt"`
	ElapsedMs       int64                   `bson:"elapsedMs"`
	Input           string                  `bson:"input"`
	Status          string                  `bson:"status"`
	Error           string                  `bson:"error"`
	ReviewError     string                  `bson:"reviewError"`
	Model           string                  `bson:"model"`
	UnsecuredStatus string                  `bson:"unsecuredStatus"`
	Crids           []string                `bson:"crids"`
	Adomains        []string                `bson:"adomains"`
	BidCount        int                     `bson:"bidCount"`
	CreativeCount   int                     `bson:"creativeCount"`
	ScoredCount     int                     `bson:"scoredCount"`
	Labels          []model.ReviewLabelStat `bson:"labels"`
	Result          *model.ReviewResult     `bson:"result,omitempty"`
}

type reviewCount struct {
	Count int `bson:"count"`
}

type reviewLabelGroup struct {
	Label string `bson:"_id"`
	Count int    `bson:"count"`
}

type reviewReasonGroup struct {
	Reason string `bson:"_id"`
	Count  int    `bson:"count"`
}

type reviewFacet struct {
	Records []reviewDocument    `bson:"records"`
	Total   []reviewCount       `bson:"total"`
	Success []reviewCount       `bson:"success"`
	Warning []reviewCount       `bson:"warning"`
	Failed  []reviewCount       `bson:"failed"`
	Hit     []reviewCount       `bson:"hit"`
	Labels  []reviewLabelGroup  `bson:"labels"`
	Reasons []reviewReasonGroup `bson:"reasons"`
}

func documentRecord(doc reviewDocument, withBody bool) model.ReviewRecord {
	crids := doc.Crids
	if crids == nil {
		crids = []string{}
	}
	adomains := doc.Adomains
	if adomains == nil {
		adomains = []string{}
	}
	labels := doc.Labels
	if labels == nil {
		labels = []model.ReviewLabelStat{}
	}
	record := model.ReviewRecord{
		ID:              doc.ID.Hex(),
		CreatedAt:       doc.CreatedAt,
		ElapsedMs:       doc.ElapsedMs,
		Status:          doc.Status,
		Error:           doc.Error,
		ReviewError:     doc.ReviewError,
		Model:           doc.Model,
		UnsecuredStatus: doc.UnsecuredStatus,
		Crids:           crids,
		Adomains:        adomains,
		BidCount:        doc.BidCount,
		CreativeCount:   doc.CreativeCount,
		ScoredCount:     doc.ScoredCount,
		Labels:          labels,
	}
	if withBody {
		record.Input = doc.Input
		record.Result = doc.Result
	}
	return record
}
