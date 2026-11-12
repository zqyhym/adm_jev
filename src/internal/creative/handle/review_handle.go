package handle

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/model"
	"adm_jev/internal/creative/service"
)

type reviewHandle struct {
	service *service.ReviewService
}

func RegisterReviewRoutes(router gin.IRouter, reviewService *service.ReviewService) {
	handle := &reviewHandle{service: reviewService}
	router.Any("/api/review", handle.review)
	if reviewService.Records != nil {
		router.GET("/api/review/records", handle.records)
		router.GET("/api/review/records/:id", handle.record)
	}
}

func (h *reviewHandle) review(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		http.Error(c.Writer, "只接受 POST", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Input string `json:"input"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20))
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体不是 JSON"})
		return
	}
	result, err := h.service.Review(c.Request.Context(), request.Input)
	if err != nil {
		body := gin.H{"error": err.Error()}
		if result.ID != "" {
			body["id"] = result.ID
		}
		c.JSON(http.StatusBadRequest, body)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *reviewHandle) records(c *gin.Context) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Printf("加载时区失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法确定时区"})
		return
	}
	fromText := strings.TrimSpace(c.Query("from"))
	toText := strings.TrimSpace(c.Query("to"))
	var fromDay, toDay time.Time
	if fromText == "" && toText == "" {
		now := time.Now().In(location)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		fromDay = today.AddDate(0, 0, -6)
		toDay = today
	} else if fromText == "" || toText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "开始日期和结束日期需要同时提供"})
		return
	} else {
		fromDay, err = time.ParseInLocation("2006-01-02", fromText, location)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "开始日期无效"})
			return
		}
		toDay, err = time.ParseInLocation("2006-01-02", toText, location)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "结束日期无效"})
			return
		}
	}
	if toDay.Before(fromDay) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "结束日期早于开始日期"})
		return
	}
	if int(toDay.Sub(fromDay).Hours()/24)+1 > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "查询跨度不能超过 90 天"})
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	switch status {
	case "", model.ReviewStatusSuccess, model.ReviewStatusWarning, model.ReviewStatusFailed:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "状态无效"})
		return
	}
	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "页码无效"})
			return
		}
	}
	pageSize := 20
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "每页条数无效"})
			return
		}
	}
	report, err := h.service.List(c.Request.Context(), model.ReviewReportQuery{
		From:     fromDay,
		To:       toDay.AddDate(0, 0, 1),
		Status:   status,
		Keyword:  strings.TrimSpace(c.Query("q")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		log.Printf("查询审核记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询审核记录失败"})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *reviewHandle) record(c *gin.Context) {
	record, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if errors.Is(err, model.ErrReviewNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "审核记录不存在"})
		return
	}
	if err != nil {
		log.Printf("读取审核记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取审核记录失败"})
		return
	}
	c.JSON(http.StatusOK, record)
}
