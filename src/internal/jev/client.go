// Package jev 调用自有 Hub 的决策接口，验证文本和图片能否被 jev 解析。
package jev

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://49.0.206.228:80"
	defaultModel   = "jev-1.13"
	decisionPath   = "/aihub/api/v1/decision"
	imageHeader    = "TypeSafe-Danger-Zone-Images"

	// 上游图片宽高和总像素的闭区间。超限会被整单拒绝。
	minImageSide  = 256
	maxImageSide  = 1568
	minImagePixel = 256 * 256
	maxImagePixel = 1568 * 1568

	StatusAnswered = "ANSWERED"
	StatusUnknown  = "UNKNOWN"
)

// Client 使用后端持有的密钥调用 Hub。
type Client struct {
	BaseURL string
	Model   string
	APIKey  string
	HTTP    *http.Client
}

// Question 是决策接口中的一个命名问题。
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// ImageInput 是一张待送审 JPEG。Ref 必须出现在 state 或题干中。
type ImageInput struct {
	Ref  string
	Data []byte
}

// Request 是一次决策请求。
type Request struct {
	State             any
	Questions         map[string]Question
	Images            []ImageInput
	EnableImageHeader bool
}

// Answer 是单个问题的解析结果。缺失时 Status 为 UNKNOWN，不表示未命中。
type Answer struct {
	Name   string
	Type   string
	Status string
	Noul   *float64
	Choice string
	Score  *float64
}

// Result 是一次成功解析的响应。Model 为空表示 Hub 未回显模型。
type Result struct {
	Model   string
	Answers map[string]Answer
}

// HTTPError 是 Hub 返回的非成功状态。Body 已脱敏并截断。
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("hub 决策请求失败: status=%d body=%s", e.Status, e.Body)
}

// MissingAnswersError 表示响应里缺少请求过的问题。
type MissingAnswersError struct {
	Names []string
}

func (e *MissingAnswersError) Error() string {
	return "缺少问题结果: " + strings.Join(e.Names, ",")
}

// NewFromEnv 读取 Hub 地址、模型和密钥。地址与模型有内网默认值。
func NewFromEnv() *Client {
	base := os.Getenv("AIHUB_BASE_URL")
	if base == "" {
		base = defaultBaseURL
	}
	model := os.Getenv("AIHUB_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Client{
		BaseURL: base,
		Model:   model,
		APIKey:  os.Getenv("AIHUB_API_KEY"),
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// ValidateImage 在发请求前拒绝非 JPEG/PNG、截断图和超限图。
func ValidateImage(data []byte) error {
	if !jpegMagic(data) && !pngMagic(data) {
		return fmt.Errorf("不是 JPEG 或 PNG")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("图片损坏或截断: %w", err)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if cfg.Width < minImageSide || cfg.Height < minImageSide || cfg.Width > maxImageSide || cfg.Height > maxImageSide || pixels < minImagePixel || pixels > maxImagePixel {
		return fmt.Errorf("图片尺寸 %dx%d 超出 %d 到 %d 像素的限制", cfg.Width, cfg.Height, minImageSide, maxImageSide)
	}
	return nil
}

// Decide 校验图片和引用后调用决策接口。
func (c *Client) Decide(ctx context.Context, req Request) (*Result, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	return c.post(ctx, req)
}

func validateRequest(req Request) error {
	if len(req.Questions) == 0 {
		return fmt.Errorf("至少需要一个问题")
	}
	encoded, err := json.Marshal(struct {
		State     any                 `json:"state"`
		Questions map[string]Question `json:"questions"`
	}{req.State, req.Questions})
	if err != nil {
		return err
	}
	text := string(encoded)
	for _, img := range req.Images {
		if img.Ref == "" {
			return fmt.Errorf("图片引用为空")
		}
		if !strings.Contains(text, img.Ref) {
			return fmt.Errorf("图片引用 %s 未出现在请求文本中", img.Ref)
		}
		if err := ValidateImage(img.Data); err != nil {
			return err
		}
	}
	return nil
}

// post 不做本地尺寸拦截，供联调观察 Hub 对超限图片的拒绝结果。
func (c *Client) post(ctx context.Context, req Request) (*Result, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("缺少 AIHUB_API_KEY")
	}
	payload, err := buildPayload(c.Model, req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	if req.EnableImageHeader {
		httpReq.Header.Set(imageHeader, "true")
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	redacted := truncate(redact(string(raw), c.APIKey), 800)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{Status: resp.StatusCode, Body: redacted}
	}
	names := make([]string, 0, len(req.Questions))
	for name := range req.Questions {
		names = append(names, name)
	}
	sort.Strings(names)
	result, err := parseResult(raw, names)
	if err != nil {
		return result, fmt.Errorf("%w; body=%s", err, redacted)
	}
	return result, nil
}

func (c *Client) endpoint() string {
	return strings.TrimRight(c.BaseURL, "/") + decisionPath
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// 组装请求体
func buildPayload(model string, req Request) ([]byte, error) {
	body := map[string]any{
		"model":     model,
		"state":     req.State,
		"questions": req.Questions,
	}
	if len(req.Images) > 0 {
		assets := make(map[string]any, len(req.Images))
		for _, img := range req.Images {
			assets[img.Ref] = map[string]any{
				"type":        "image",
				"format":      imageFormat(img.Data),
				"data_base64": base64.StdEncoding.EncodeToString(img.Data),
			}
		}
		body["assets"] = assets
	}
	return json.Marshal(body)
}

func parseResult(raw []byte, names []string) (*Result, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("响应不是 JSON: %w", err)
	}
	answersRaw, ok := findAnswers(root)
	if !ok {
		return nil, fmt.Errorf("响应中没有 answers")
	}
	result := &Result{
		Model:   asString(root["model"]),
		Answers: make(map[string]Answer, len(names)),
	}
	if result.Model == "" {
		if data, ok := root["data"].(map[string]any); ok {
			result.Model = asString(data["model"])
		}
	}
	var missing []string
	for _, name := range names {
		item, exists := answersRaw[name]
		if !exists {
			result.Answers[name] = Answer{Name: name, Status: StatusUnknown}
			missing = append(missing, name)
			continue
		}
		answer, ok := parseAnswer(name, item)
		if !ok {
			result.Answers[name] = Answer{Name: name, Status: StatusUnknown}
			missing = append(missing, name)
			continue
		}
		result.Answers[name] = answer
	}
	if len(missing) > 0 {
		return result, &MissingAnswersError{Names: missing}
	}
	return result, nil
}

func findAnswers(root map[string]any) (map[string]any, bool) {
	if answers, ok := asObject(root["answers"]); ok {
		return answers, true
	}
	for _, key := range []string{"data", "result"} {
		if nested, ok := root[key].(map[string]any); ok {
			if answers, ok := asObject(nested["answers"]); ok {
				return answers, true
			}
		}
	}
	return nil, false
}

func parseAnswer(name string, item any) (Answer, bool) {
	obj, ok := item.(map[string]any)
	if !ok {
		return Answer{}, false
	}
	answer := Answer{Name: name, Type: asString(obj["type"]), Status: StatusAnswered}
	switch answer.Type {
	case "noul":
		value, ok := asFloat(obj["noul"])
		if !ok || value < 0 || value > 1 {
			return Answer{}, false
		}
		answer.Noul = &value
	case "choice":
		answer.Choice = asString(obj["choice"])
		if answer.Choice == "" {
			return Answer{}, false
		}
	case "score":
		value, ok := asFloat(obj["score"])
		if !ok {
			return Answer{}, false
		}
		answer.Score = &value
	default:
		return Answer{}, false
	}
	return answer, true
}

func asObject(v any) (map[string]any, bool) {
	obj, ok := v.(map[string]any)
	return obj, ok
}

func asString(v any) string {
	text, _ := v.(string)
	return text
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func jpegMagic(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}

func pngMagic(data []byte) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}

func imageFormat(data []byte) string {
	if pngMagic(data) {
		return "png"
	}
	return "jpg"
}

func redact(body, key string) string {
	if key == "" {
		return body
	}
	return strings.ReplaceAll(body, key, "[REDACTED]")
}

func truncate(body string, limit int) string {
	if len(body) <= limit {
		return body
	}
	return body[:limit]
}
