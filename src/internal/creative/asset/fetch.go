package asset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"adm_jev/internal/creative/model"
)

const (
	minImageSide = 280
	maxImageSide = 1568
	maxBodyBytes = 8 << 20
)

// Fetcher 下载创意图片。AllowHost 为空时拒绝本机、私网和元数据地址。
type Fetcher struct {
	HTTP      *http.Client
	AllowHost func(host string) error
	MaxBytes  int64
}

// Fetch 只下载 kind=image 且 role=creative 的来源。追踪像素不会发出请求。
func Fetch(ctx context.Context, bids []model.BidSource, fetcher *Fetcher) map[string][]byte {
	if fetcher == nil {
		fetcher = &Fetcher{}
	}
	files := map[string][]byte{}
	for i := range bids {
		for j := range bids[i].Assets {
			asset := &bids[i].Assets[j]
			if asset.Kind != "image" || asset.Role != "creative" {
				continue
			}
			data, err := fetcher.get(ctx, asset.Value)
			if err != nil {
				asset.Obtained = false
				asset.FetchError = err.Error()
				continue
			}
			sum := sha256.Sum256(data)
			asset.ContentHash = hex.EncodeToString(sum[:])
			asset.Obtained = true
			asset.FetchError = ""
			files[asset.SourceID] = data
		}
	}
	return files
}

func (f *Fetcher) get(ctx context.Context, rawURL string) ([]byte, error) {
	if err := f.allow(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败: status=%d", resp.StatusCode)
	}
	limit := f.MaxBytes
	if limit <= 0 {
		limit = maxBodyBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("图片超过大小限制")
	}
	if err := usableImage(data); err != nil {
		return nil, err
	}
	return data, nil
}

func (f *Fetcher) allow(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.Contains(rawURL, "${") {
		return fmt.Errorf("地址无法下载")
	}
	check := f.AllowHost
	if check == nil {
		check = RejectRestrictedHost
	}
	if err := check(parsed.Hostname()); err != nil {
		return err
	}
	return nil
}

func (f *Fetcher) client() *http.Client {
	base := f.HTTP
	if base == nil {
		base = &http.Client{Timeout: 15 * time.Second}
	}
	clone := *base
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("重定向次数过多")
		}
		return f.allow(req.URL.String())
	}
	return &clone
}

// RejectRestrictedHost 拒绝本机、链路本地、私网和云元数据地址。
func RejectRestrictedHost(host string) error {
	if strings.EqualFold(host, "metadata.google.internal") {
		return fmt.Errorf("目标地址被拒绝")
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil || len(resolved) == 0 {
			return fmt.Errorf("目标地址被拒绝")
		}
		ips = resolved
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("目标地址被拒绝")
		}
	}
	return nil
}

func usableImage(data []byte) error {
	if !jpegMagic(data) && !pngMagic(data) {
		return fmt.Errorf("不是 JPEG 或 PNG")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("图片损坏或截断")
	}
	if cfg.Width < minImageSide || cfg.Height < minImageSide || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return fmt.Errorf("图片尺寸 %dx%d 超出 %d 到 %d", cfg.Width, cfg.Height, minImageSide, maxImageSide)
	}
	return nil
}

func jpegMagic(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
}

func pngMagic(data []byte) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
}
