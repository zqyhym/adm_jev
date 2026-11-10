package handle

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/service"
)

type reviewHandle struct {
	service *service.ReviewService
}

func RegisterReviewRoutes(router gin.IRouter, reviewService *service.ReviewService) {
	handle := &reviewHandle{service: reviewService}
	router.Any("/api/review", handle.review)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
