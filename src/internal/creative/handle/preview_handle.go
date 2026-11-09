package handle

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"adm_jev/internal/creative/preview"
	"adm_jev/internal/creative/service"
)

type previewHandle struct {
	service *service.PreviewService
}

func RegisterRoutes(router gin.IRouter, previewService *service.PreviewService) {
	handle := &previewHandle{service: previewService}
	router.Any("/api/preview", handle.preview)
	router.Any("/api/empty", handle.empty)
	router.Any("/mraid.js", handle.mraid)
}

func (h *previewHandle) preview(c *gin.Context) {
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
	result, err := h.service.Preview(request.Input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *previewHandle) empty(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "image/gif", []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	})
}

func (h *previewHandle) mraid(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/javascript; charset=utf-8", []byte(preview.MraidJS()))
}
