// 离线评测静态图片：复用生产链路取素材和送审，把 noul 分成三态并统计分布与重复运行稳定性。
// 真值来自 datasets 的 <label>_pos/_neg 目录，只约束该目录对应的标签，其余标签视为未标注。
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"adm_jev/internal/creative/asset"
	"adm_jev/internal/jev"
	"adm_jev/internal/material/fetch"
	"adm_jev/internal/material/parser"
)

const (
	bucketHit       = "HIT"
	bucketNotHit    = "NOT_HIT"
	bucketUncertain = "UNCERTAIN"
	bucketUnknown   = "UNKNOWN"
)

type sample struct {
	SampleID string `json:"sampleId"`
	Image    string `json:"image"`
	Copy     string `json:"copy"`
	GTLabel  string `json:"gtLabel,omitempty"`
	GTPos    bool   `json:"gtPos,omitempty"`
}

type imageData struct {
	data   []byte
	ext    string
	width  int
	height int
	hash   string
}

type labelRun struct {
	Label         string   `json:"label"`
	Status        string   `json:"status"`
	Bucket        string   `json:"bucket"`
	Reason        string   `json:"reason,omitempty"`
	UpstreamModel string   `json:"upstreamModel,omitempty"`
	Noul          *float64 `json:"noul,omitempty"`
}

type record struct {
	SampleID      string     `json:"sampleId"`
	Repeat        int        `json:"repeat"`
	AssetHash     string     `json:"assetHash"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	Obtained      bool       `json:"obtained"`
	SkipReason    string     `json:"skipReason,omitempty"`
	FetchError    string     `json:"fetchError,omitempty"`
	ParseStatus   string     `json:"parseStatus"`
	Unsecured     string     `json:"unsecured"`
	Model         string     `json:"model"`
	UpstreamModel string     `json:"upstreamModel,omitempty"`
	PromptVersion string     `json:"promptVersion"`
	ElapsedMs     int64      `json:"elapsedMs"`
	ReviewError   string     `json:"reviewError,omitempty"`
	Labels        []labelRun `json:"labels"`
}

type evaluator struct {
	client        *jev.Client
	serverURL     string
	images        map[string]imageData
	hitThreshold  float64
	notThreshold  float64
	promptVersion string
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	manifest := flag.String("manifest", "", "样本清单 jsonl，路径相对仓库根；设置后优先于 -scan")
	scan := flag.String("scan", "datasets/static-v1/images", "扫描 <label>_pos/_neg 目录推导样本与单标签真值")
	out := flag.String("out", "eval-runs/runs.jsonl", "逐次运行结果 JSONL")
	report := flag.String("report", "eval-runs/report.md", "汇总报告 Markdown")
	repeat := flag.Int("repeat", 1, "每个样本重复运行次数")
	hit := flag.Float64("hit", 0.8, "noul 大于等于该值判为 HIT")
	notHit := flag.Float64("not-hit", 0.2, "noul 小于等于该值判为 NOT_HIT")
	sweep := flag.Bool("sweep", false, "输出 per-label 阈值扫描候选（校准用，样本充足前默认关闭）")
	flag.Parse()

	if *repeat < 1 {
		return fmt.Errorf("repeat 至少为 1")
	}
	if *notHit < 0 || *hit > 1 || *notHit >= *hit {
		return fmt.Errorf("阈值需满足 0 <= not-hit < hit <= 1")
	}
	if strings.TrimSpace(os.Getenv("AIHUB_API_KEY")) == "" {
		return fmt.Errorf("缺少 AIHUB_API_KEY，无法送审")
	}

	root := repoRoot()
	var samples []sample
	var err error
	source := ""
	if *manifest != "" {
		source = *manifest
		samples, err = loadSamples(filepath.Join(root, *manifest))
	} else {
		source = "scan:" + *scan
		samples, err = scanSamples(root, *scan)
	}
	if err != nil {
		return err
	}
	images, err := loadImages(root, samples)
	if err != nil {
		return err
	}
	prompt, err := promptVersion(root)
	if err != nil {
		return err
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(path.Base(r.URL.Path), path.Ext(r.URL.Path))
		img, ok := images[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(img.data))
		w.Write(img.data)
	}))
	defer server.Close()

	e := &evaluator{
		client:        jev.NewFromEnv(),
		serverURL:     server.URL,
		images:        images,
		hitThreshold:  *hit,
		notThreshold:  *notHit,
		promptVersion: prompt,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, *out)), 0o755); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(root, *out))
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)

	var records []record
	for round := 1; round <= *repeat; round++ {
		for _, s := range samples {
			if ctx.Err() != nil {
				break
			}
			rec := e.once(ctx, s, round)
			if err := encoder.Encode(rec); err != nil {
				return err
			}
			records = append(records, rec)
			log.Printf("round=%d sample=%s elapsed=%dms hit=%s uncertain=%d unknown=%d err=%s",
				round, rec.SampleID, rec.ElapsedMs, strings.Join(hitLabels(rec), ","),
				countBucket(rec, bucketUncertain), countBucket(rec, bucketUnknown), rec.ReviewError)
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("没有运行结果")
	}

	text := buildReport(records, samples, source, *repeat, *hit, *notHit, *sweep, e.client.Model, prompt)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, *report)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, *report), []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Print(summary(records, samples, *repeat, *hit, *notHit))
	log.Printf("结果写入 %s，报告写入 %s", *out, *report)
	return nil
}

func (e *evaluator) once(ctx context.Context, s sample, round int) record {
	started := time.Now()
	img := e.images[s.SampleID]
	rec := record{
		SampleID:      s.SampleID,
		Repeat:        round,
		AssetHash:     img.hash,
		Width:         img.width,
		Height:        img.height,
		Model:         e.client.Model,
		PromptVersion: e.promptVersion,
		Labels:        []labelRun{},
	}
	finish := func() record {
		rec.ElapsedMs = time.Since(started).Milliseconds()
		return rec
	}

	adm := ""
	if strings.TrimSpace(s.Copy) != "" {
		adm = "<div>" + html.EscapeString(s.Copy) + "</div>"
	}
	payload := map[string]any{"seatbid": []any{map[string]any{"bid": []any{map[string]any{
		"crid": s.SampleID,
		"adm":  adm,
		"iurl": e.serverURL + "/" + s.SampleID + img.ext,
	}}}}}
	raw, err := json.Marshal(payload)
	if err != nil {
		rec.ReviewError = err.Error()
		return finish()
	}
	bids, err := parser.Parse(string(raw))
	if err != nil {
		rec.ReviewError = err.Error()
		return finish()
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	files := fetch.Fetch(ctx, bids, &fetch.Fetcher{AllowHost: allowLocal})
	sourceID := ""
	for _, bid := range bids {
		rec.ParseStatus = bid.ParseStatus
		for _, item := range bid.Assets {
			if item.Kind != "image" || item.Role != "creative" {
				continue
			}
			sourceID = item.SourceID
			rec.Obtained = item.Obtained
			if item.ContentHash != "" {
				rec.AssetHash = item.ContentHash
			}
			rec.FetchError = item.FetchError
		}
	}
	rec.Unsecured = asset.UnsecuredRequest(bids).Status

	labels, reviewErr := asset.Submit(ctx, e.client, bids, files)
	if reviewErr != nil {
		rec.ReviewError = reviewErr.Error()
	}
	for _, label := range labels {
		if label.SourceID != sourceID {
			continue
		}
		rec.Labels = append(rec.Labels, labelRun{
			Label:         label.Label,
			Status:        label.Status,
			Bucket:        e.bucket(label),
			Reason:        label.Reason,
			UpstreamModel: label.UpstreamModel,
			Noul:          label.Noul,
		})
		if rec.UpstreamModel == "" {
			rec.UpstreamModel = label.UpstreamModel
		}
	}
	rec.SkipReason = materialReason(rec.Labels)
	return finish()
}

// materialReason 只取素材层原因，模型层失败不算未送审。
func materialReason(labels []labelRun) string {
	for _, label := range labels {
		switch label.Reason {
		case asset.ReasonNotObtained, asset.ReasonNoData, asset.ReasonImageSize, asset.ReasonInvalidImage, asset.ReasonNoSubject:
			return label.Reason
		}
	}
	return ""
}

func (e *evaluator) bucket(label asset.LabelResult) string {
	if label.Status != asset.StatusScored || label.Noul == nil {
		return bucketUnknown
	}
	switch {
	case *label.Noul >= e.hitThreshold:
		return bucketHit
	case *label.Noul <= e.notThreshold:
		return bucketNotHit
	default:
		return bucketUncertain
	}
}

func allowLocal(host string) error {
	if host == "127.0.0.1" {
		return nil
	}
	return fmt.Errorf("评测只允许本地样本服务: %s", host)
}

func loadSamples(path string) ([]sample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var samples []sample
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		var s sample
		if err := json.Unmarshal([]byte(text), &s); err != nil {
			return nil, fmt.Errorf("清单第 %d 行不是 JSON: %w", line, err)
		}
		if s.SampleID == "" || s.Image == "" {
			return nil, fmt.Errorf("清单第 %d 行缺少 sampleId 或 image", line)
		}
		if strings.ContainsAny(s.SampleID, "/\\?#% ") || strings.Contains(s.SampleID, "..") {
			return nil, fmt.Errorf("清单第 %d 行 sampleId 含非法字符: %s", line, s.SampleID)
		}
		if seen[s.SampleID] {
			return nil, fmt.Errorf("清单第 %d 行 sampleId 重复: %s", line, s.SampleID)
		}
		seen[s.SampleID] = true
		samples = append(samples, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("清单为空: %s", path)
	}
	return samples, nil
}

// folderLabels 把 datasets 目录名映射到生产标签名，需与 asset.visualLabels 保持一致。
var folderLabels = map[string]string{
	"adultent":   "Adult Entertainment",
	"adultprod":  "Adult Products",
	"dating":     "Dating",
	"gambling":   "Gambling",
	"marijuana":  "Marijuana",
	"nudity":     "Nudity",
	"profane":    "Profane Language",
	"religion":   "Religion Spirituality",
	"suggestive": "Sexually Suggestive",
	"violence":   "Violence",
}

// scanSamples 遍历 <label>_pos/_neg 目录，目录名即该标签的真值；其余标签视为未标注。
func scanSamples(root, dir string) ([]sample, error) {
	base := filepath.Join(root, filepath.FromSlash(dir))
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var samples []sample
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		key, polarity, ok := strings.Cut(name, "_")
		if !ok || (polarity != "pos" && polarity != "neg") {
			return nil, fmt.Errorf("目录 %s 不符合 <label>_pos|_neg 命名", name)
		}
		label, ok := folderLabels[key]
		if !ok {
			return nil, fmt.Errorf("目录 %s 的标签键 %s 未知", name, key)
		}
		files, err := os.ReadDir(filepath.Join(base, name))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(f.Name()))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}
			samples = append(samples, sample{
				SampleID: name + "_" + strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())),
				Image:    filepath.ToSlash(filepath.Join(dir, name, f.Name())),
				GTLabel:  label,
				GTPos:    polarity == "pos",
			})
		}
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("扫描目录为空: %s", dir)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].SampleID < samples[j].SampleID })
	return samples, nil
}

func loadImages(root string, samples []sample) (map[string]imageData, error) {
	images := map[string]imageData{}
	for _, s := range samples {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(s.Image)))
		if err != nil {
			return nil, fmt.Errorf("读取样本 %s: %w", s.SampleID, err)
		}
		cfg, _, err := image.DecodeConfig(strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("样本 %s 无法解码: %w", s.SampleID, err)
		}
		sum := sha256.Sum256(data)
		images[s.SampleID] = imageData{
			data:   data,
			ext:    strings.ToLower(filepath.Ext(s.Image)),
			width:  cfg.Width,
			height: cfg.Height,
			hash:   hex.EncodeToString(sum[:]),
		}
	}
	return images, nil
}

// promptVersion 用送审代码的文件 hash 代理：标签表或题干改动都会改变它。
func promptVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "src", "internal", "creative", "asset", "review.go"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12], nil
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func hitLabels(rec record) []string {
	var names []string
	for _, label := range rec.Labels {
		if label.Bucket == bucketHit {
			names = append(names, label.Label)
		}
	}
	return names
}

func countBucket(rec record, bucket string) int {
	n := 0
	for _, label := range rec.Labels {
		if label.Bucket == bucket {
			n++
		}
	}
	return n
}

type labelStats struct {
	label                                  string
	total, hit, notHit, uncertain, unknown int
	sum, min, max                          float64
	scored                                 int
}

func distribution(records []record) []labelStats {
	byLabel := map[string]*labelStats{}
	var order []string
	for _, rec := range records {
		if rec.Repeat != 1 {
			continue
		}
		for _, label := range rec.Labels {
			stat, ok := byLabel[label.Label]
			if !ok {
				stat = &labelStats{label: label.Label, min: 1, max: 0}
				byLabel[label.Label] = stat
				order = append(order, label.Label)
			}
			stat.total++
			switch label.Bucket {
			case bucketHit:
				stat.hit++
			case bucketNotHit:
				stat.notHit++
			case bucketUncertain:
				stat.uncertain++
			default:
				stat.unknown++
			}
			if label.Noul != nil {
				value := *label.Noul
				if stat.scored == 0 || value < stat.min {
					stat.min = value
				}
				if stat.scored == 0 || value > stat.max {
					stat.max = value
				}
				stat.scored++
				stat.sum += value
			}
		}
	}
	sort.Strings(order)
	stats := make([]labelStats, 0, len(order))
	for _, name := range order {
		stats = append(stats, *byLabel[name])
	}
	return stats
}

type stability struct {
	sample  string
	label   string
	runs    int
	mean    float64
	std     float64
	spread  float64
	buckets int
}

func stabilityGroups(records []record) []stability {
	type key struct{ sample, label string }
	values := map[key][]float64{}
	buckets := map[key]map[string]bool{}
	var order []key
	for _, rec := range records {
		for _, label := range rec.Labels {
			k := key{rec.SampleID, label.Label}
			if _, ok := values[k]; !ok {
				order = append(order, k)
			}
			if buckets[k] == nil {
				buckets[k] = map[string]bool{}
			}
			buckets[k][label.Bucket] = true
			if label.Noul != nil {
				values[k] = append(values[k], *label.Noul)
			}
		}
	}
	var groups []stability
	for _, k := range order {
		vs := values[k]
		if len(vs) < 2 {
			continue
		}
		mean := 0.0
		for _, v := range vs {
			mean += v
		}
		mean /= float64(len(vs))
		variance, min, max := 0.0, vs[0], vs[0]
		for _, v := range vs {
			variance += (v - mean) * (v - mean)
			min = math.Min(min, v)
			max = math.Max(max, v)
		}
		groups = append(groups, stability{
			sample:  k.sample,
			label:   k.label,
			runs:    len(vs),
			mean:    mean,
			std:     math.Sqrt(variance / float64(len(vs))),
			spread:  max - min,
			buckets: len(buckets[k]),
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].spread > groups[j].spread })
	return groups
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// gtStats 是单个标签在基线阈值下的混淆计数。真值只约束该标签，abstain 表示 UNCERTAIN/UNKNOWN。
type gtStats struct {
	label                  string
	nPos, nNeg             int
	tp, fp, fn, tn         int
	abstainPos, abstainNeg int
}

func (g gtStats) precision() float64 {
	if g.tp+g.fp == 0 {
		return -1
	}
	return float64(g.tp) / float64(g.tp+g.fp)
}

func (g gtStats) recallDecided() float64 {
	if g.tp+g.fn == 0 {
		return -1
	}
	return float64(g.tp) / float64(g.tp+g.fn)
}

func (g gtStats) recallAll() float64 {
	if g.nPos == 0 {
		return -1
	}
	return float64(g.tp) / float64(g.nPos)
}

func (g gtStats) fnr() float64 {
	if g.tp+g.fn == 0 {
		return -1
	}
	return float64(g.fn) / float64(g.tp+g.fn)
}

func groundTruth(records []record, samples []sample) []gtStats {
	gt := map[string]sample{}
	for _, s := range samples {
		if s.GTLabel != "" {
			gt[s.SampleID] = s
		}
	}
	byLabel := map[string]*gtStats{}
	var order []string
	for _, rec := range firstRound(records) {
		s, ok := gt[rec.SampleID]
		if !ok {
			continue
		}
		for _, label := range rec.Labels {
			if label.Label != s.GTLabel {
				continue
			}
			stat, ok := byLabel[label.Label]
			if !ok {
				stat = &gtStats{label: label.Label}
				byLabel[label.Label] = stat
				order = append(order, label.Label)
			}
			if s.GTPos {
				stat.nPos++
			} else {
				stat.nNeg++
			}
			switch label.Bucket {
			case bucketHit:
				if s.GTPos {
					stat.tp++
				} else {
					stat.fp++
				}
			case bucketNotHit:
				if s.GTPos {
					stat.fn++
				} else {
					stat.tn++
				}
			default:
				if s.GTPos {
					stat.abstainPos++
				} else {
					stat.abstainNeg++
				}
			}
		}
	}
	sort.Strings(order)
	out := make([]gtStats, 0, len(order))
	for _, name := range order {
		out = append(out, *byLabel[name])
	}
	return out
}

type sweepPoint struct {
	threshold             float64
	precision, recall, f1 float64
	tp, fp, fn, tn        int
}

// sweepLabel 对单个标签做二值阈值扫描（noul>=t 判正，否则判负，不含 abstain）。
func sweepLabel(records []record, samples []sample, label string) []sweepPoint {
	gt := map[string]bool{}
	for _, s := range samples {
		if s.GTLabel == label {
			gt[s.SampleID] = s.GTPos
		}
	}
	type pair struct {
		score float64
		pos   bool
	}
	var pairs []pair
	for _, rec := range firstRound(records) {
		pos, ok := gt[rec.SampleID]
		if !ok {
			continue
		}
		for _, l := range rec.Labels {
			if l.Label == label && l.Noul != nil {
				pairs = append(pairs, pair{*l.Noul, pos})
			}
		}
	}
	var points []sweepPoint
	for i := 1; i <= 19; i++ {
		t := float64(i) / 20
		var tp, fp, fn, tn int
		for _, p := range pairs {
			if p.score >= t {
				if p.pos {
					tp++
				} else {
					fp++
				}
			} else if p.pos {
				fn++
			} else {
				tn++
			}
		}
		prec, rec, f1 := -1.0, -1.0, -1.0
		if tp+fp > 0 {
			prec = float64(tp) / float64(tp+fp)
		}
		if tp+fn > 0 {
			rec = float64(tp) / float64(tp+fn)
		}
		if prec >= 0 && rec >= 0 && prec+rec > 0 {
			f1 = 2 * prec * rec / (prec + rec)
		}
		points = append(points, sweepPoint{threshold: t, precision: prec, recall: rec, f1: f1, tp: tp, fp: fp, fn: fn, tn: tn})
	}
	return points
}

// candidates 挑三个候选工作点：最佳 F1、precision>=0.85 下最高 recall、recall>=0.90 下最高 precision。
func candidates(points []sweepPoint) (bestF1, highRecall, highPrecision *sweepPoint) {
	for i := range points {
		p := &points[i]
		if p.f1 >= 0 && (bestF1 == nil || p.f1 > bestF1.f1) {
			bestF1 = p
		}
		if p.precision >= 0.85 && p.recall >= 0 && (highRecall == nil || p.recall > highRecall.recall) {
			highRecall = p
		}
		if p.recall >= 0.90 && p.precision >= 0 && (highPrecision == nil || p.precision > highPrecision.precision) {
			highPrecision = p
		}
	}
	return
}

func pctOrDash(v float64) string {
	if v < 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

func buildReport(records []record, samples []sample, source string, repeat int, hit, notHit float64, sweepOn bool, model, prompt string) string {
	var b strings.Builder
	first := firstRound(records)
	stats := distribution(records)
	gt := groundTruth(records, samples)

	title := "静态图片离线评测报告"
	if len(gt) == 0 {
		title += "（无真值）"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- 生成时间：%s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "- 样本来源：`%s`，样本 %d 个，重复 %d 次，运行记录 %d 条\n", source, len(samples), repeat, len(records))
	fmt.Fprintf(&b, "- 判定口径：`noul >= %.2f` HIT，`noul <= %.2f` NOT_HIT，其余 UNCERTAIN；模型缺项或素材未送审为 UNKNOWN\n", hit, notHit)
	upstream := "(未回显)"
	for _, rec := range records {
		if rec.UpstreamModel != "" {
			upstream = rec.UpstreamModel
			break
		}
	}
	fmt.Fprintf(&b, "- 请求模型：`%s`；Hub 回显上游：`%s`；promptVersion：`%s`（送审代码文件 hash 代理）\n\n", model, upstream, prompt)

	fmt.Fprintf(&b, "## 1. 素材送审情况\n\n")
	obtained, sent, skipped := 0, 0, 0
	reasons := map[string]int{}
	for _, rec := range first {
		if rec.Obtained {
			obtained++
		}
		if rec.SkipReason != "" {
			skipped++
			reasons[rec.SkipReason]++
			continue
		}
		if rec.Obtained {
			sent++
		}
	}
	fmt.Fprintf(&b, "| 项 | 数量 | 占比 |\n| --- | --- | --- |\n")
	fmt.Fprintf(&b, "| 样本总数 | %d | 100%% |\n", len(first))
	fmt.Fprintf(&b, "| 下载成功 | %d | %s |\n", obtained, ratio(obtained, len(first)))
	fmt.Fprintf(&b, "| 实际送审 | %d | %s |\n", sent, ratio(sent, len(first)))
	fmt.Fprintf(&b, "| 未送审 | %d | %s |\n\n", skipped, ratio(skipped, len(first)))
	if len(reasons) > 0 {
		fmt.Fprintf(&b, "未送审归因：\n\n")
		for reason, n := range reasons {
			fmt.Fprintf(&b, "- `%s`：%d\n", reason, n)
		}
		fmt.Fprintf(&b, "\n")
	}
	parseStatus := map[string]int{}
	unsecured := map[string]int{}
	for _, rec := range first {
		parseStatus[rec.ParseStatus]++
		unsecured[rec.Unsecured]++
	}
	fmt.Fprintf(&b, "解析状态：%s。Unsecured Request：%s。\n\n", joinCounts(parseStatus), joinCounts(unsecured))

	unknownReasons := map[string]int{}
	for _, rec := range first {
		for _, label := range rec.Labels {
			if label.Bucket == bucketUnknown {
				unknownReasons[label.Reason]++
			}
		}
	}
	fmt.Fprintf(&b, "UNKNOWN 归因（首轮，按标签行计）：%s。\n\n", joinCounts(unknownReasons))
	fmt.Fprintf(&b, "归因口径：素材层 `NOT_OBTAINED` 下载失败、`NO_DATA` 无字节、`IMAGE_SIZE` 尺寸超限、`INVALID_IMAGE` 格式不支持或损坏、`NO_SUBJECT` 无主体图；模型层 `HUB_FAILED` 请求失败、`MISSING_ANSWER` 标签缺项或分数无效。\n\n")

	fmt.Fprintf(&b, "## 2. 分标签三态分布（首轮）\n\n")
	fmt.Fprintf(&b, "| 标签 | 样本数 | HIT | NOT_HIT | UNCERTAIN | UNKNOWN | 可判定率 | noul 均值 | noul 范围 |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	totalHit, totalNotHit, totalUncertain, totalUnknown, totalAll := 0, 0, 0, 0, 0
	for _, stat := range stats {
		mean, spread := "-", "-"
		if stat.scored > 0 {
			mean = fmt.Sprintf("%.3f", stat.sum/float64(stat.scored))
			spread = fmt.Sprintf("%.2f–%.2f", stat.min, stat.max)
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %s | %s | %s |\n",
			stat.label, stat.total, stat.hit, stat.notHit, stat.uncertain, stat.unknown,
			ratio(stat.hit+stat.notHit, stat.total), mean, spread)
		totalHit += stat.hit
		totalNotHit += stat.notHit
		totalUncertain += stat.uncertain
		totalUnknown += stat.unknown
		totalAll += stat.total
	}
	fmt.Fprintf(&b, "| **合计** | %d | %d | %d | %d | %d | %s | - | - |\n\n",
		totalAll, totalHit, totalNotHit, totalUncertain, totalUnknown, ratio(totalHit+totalNotHit, totalAll))
	fmt.Fprintf(&b, "可判定率 = (HIT + NOT_HIT) / 全部标签结果。UNKNOWN 率 = %s。\n\n", ratio(totalUnknown, totalAll))

	if len(gt) > 0 {
		fmt.Fprintf(&b, "## 3. 真值指标（基线阈值 %.2f/%.2f）\n\n", hit, notHit)
		fmt.Fprintf(&b, "真值只约束样本目录对应的标签，其余标签未标注、不计入。abstain = UNCERTAIN/UNKNOWN，送复核而非自动放行。\n")
		fmt.Fprintf(&b, "阈值 %.2f/%.2f 为暂定工作点（未经校准）；本节指标只描述分数在该工作点的落点，不代表已验证的准确率。样本量小，逐标签计数仅供观察。\n\n", hit, notHit)
		fmt.Fprintf(&b, "| 标签 | 正/负 | TP | FP | FN | TN | abstain正/负 | Precision | Recall(已判) | Recall(全部正) | FNR(已判) |\n")
		fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, g := range gt {
			fmt.Fprintf(&b, "| %s | %d/%d | %d | %d | %d | %d | %d/%d | %s | %s | %s | %s |\n",
				g.label, g.nPos, g.nNeg, g.tp, g.fp, g.fn, g.tn, g.abstainPos, g.abstainNeg,
				pctOrDash(g.precision()), pctOrDash(g.recallDecided()), pctOrDash(g.recallAll()), pctOrDash(g.fnr()))
		}
		fmt.Fprintf(&b, "\nRecall(已判) 分母为 TP+FN，不含 abstain；Recall(全部正) 分母为全部正例，abstain 计为未召回。无正例的标签 recall 记为 -。\n\n")

	}

	if len(gt) > 0 && sweepOn {
		fmt.Fprintf(&b, "## 4. 阈值扫描候选（二值，不含 abstain）\n\n")
		fmt.Fprintf(&b, "| 标签 | 候选 | 阈值 | Precision | Recall | F1 |\n| --- | --- | --- | --- | --- | --- |\n")
		for _, g := range gt {
			points := sweepLabel(records, samples, g.label)
			bestF1, highRecall, highPrecision := candidates(points)
			rows := []struct {
				name string
				p    *sweepPoint
			}{{"最佳F1", bestF1}, {"P>=85%最高R", highRecall}, {"R>=90%最高P", highPrecision}}
			for _, row := range rows {
				if row.p == nil {
					fmt.Fprintf(&b, "| %s | %s | - | - | - | - |\n", g.label, row.name)
					continue
				}
				fmt.Fprintf(&b, "| %s | %s | %.2f | %s | %s | %s |\n", g.label, row.name, row.p.threshold,
					pctOrDash(row.p.precision), pctOrDash(row.p.recall), pctOrDash(row.p.f1))
			}
		}
		fmt.Fprintf(&b, "\n扫描步长 0.05；候选仅供选择，最终阈值需在独立验证集确认。\n\n")
	}

	fmt.Fprintf(&b, "## 5. 每样本命中概览（首轮）\n\n")
	fmt.Fprintf(&b, "| 样本 | 尺寸 | HIT 标签 | UNCERTAIN | UNKNOWN | Unsecured | 耗时 ms |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, rec := range first {
		hits := strings.Join(hitLabels(rec), ", ")
		if hits == "" {
			hits = "-"
		}
		fmt.Fprintf(&b, "| %s | %dx%d | %s | %d | %d | %s | %d |\n",
			rec.SampleID, rec.Width, rec.Height, hits, countBucket(rec, bucketUncertain),
			countBucket(rec, bucketUnknown), rec.Unsecured, rec.ElapsedMs)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## 6. 重复运行稳定性\n\n")
	groups := stabilityGroups(records)
	if len(groups) == 0 {
		fmt.Fprintf(&b, "repeat=1，未测量稳定性。用 `-repeat 5` 重跑本节。\n\n")
	} else {
		flipped, stdSum, maxSpread := 0, 0.0, 0.0
		for _, g := range groups {
			if g.buckets > 1 {
				flipped++
			}
			stdSum += g.std
			maxSpread = math.Max(maxSpread, g.spread)
		}
		fmt.Fprintf(&b, "- 可比较组数（样本 × 标签，至少 2 次有分数）：%d\n", len(groups))
		fmt.Fprintf(&b, "- **跨桶翻转率：%s**（%d 组在 HIT / NOT_HIT / UNCERTAIN / UNKNOWN 之间变化）\n", ratio(flipped, len(groups)), flipped)
		fmt.Fprintf(&b, "- 平均标准差：%.4f；最大极差：%.2f\n\n", stdSum/float64(len(groups)), maxSpread)
		fmt.Fprintf(&b, "极差最大的 10 组：\n\n| 样本 | 标签 | 次数 | 均值 | 标准差 | 极差 | 桶数 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
		for i, g := range groups {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %.3f | %.4f | %.2f | %d |\n", g.sample, g.label, g.runs, g.mean, g.std, g.spread, g.buckets)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## 7. 耗时\n\n")
	var elapsed []int64
	for _, rec := range records {
		elapsed = append(elapsed, rec.ElapsedMs)
	}
	fmt.Fprintf(&b, "单样本端到端（含下载与一次 10 问决策）：P50 %d ms，P95 %d ms，最大 %d ms。\n\n",
		percentile(elapsed, 0.5), percentile(elapsed, 0.95), percentile(elapsed, 1))

	fmt.Fprintf(&b, "## 8. 口径与局限\n\n")
	if len(gt) > 0 {
		fmt.Fprintf(&b, "- 真值来自 datasets 目录 pos/neg，只约束单标签；跨标签共现未标注、不计入指标。目录真值未经人工复核，不等同业务 Ground Truth。\n")
	} else {
		fmt.Fprintf(&b, "- 本轮无真值，**不报准确率、Precision、Recall**。三态分布只说明分数落点，不说明判对。\n")
	}
	fmt.Fprintf(&b, "- 阈值 %.2f / %.2f 是全局暂定值，未按标签校准；中间带宽度直接决定可判定率。\n", hit, notHit)
	fmt.Fprintf(&b, "- 上游模型取 Hub 回显（逐标签携带，记录级取首个非空）；为空表示 Hub 未回显，不能据此认定上游版本未变。\n")
	fmt.Fprintf(&b, "- promptVersion 是送审代码文件 hash，标签定义尚未由业务冻结为独立 labelVersion。\n")
	fmt.Fprintf(&b, "- 分布统计只取首轮，避免重复运行放大样本量；稳定性统计使用全部轮次。\n")
	var errors []string
	for _, rec := range records {
		if rec.ReviewError != "" {
			errors = append(errors, fmt.Sprintf("%s#%d: %s", rec.SampleID, rec.Repeat, rec.ReviewError))
		}
	}
	if len(errors) > 0 {
		fmt.Fprintf(&b, "\n运行错误：\n\n")
		for _, item := range errors {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	return b.String()
}

func summary(records []record, samples []sample, repeat int, hit, notHit float64) string {
	first := firstRound(records)
	stats := distribution(records)
	hitTotal, notTotal, uncertainTotal, unknownTotal, all := 0, 0, 0, 0, 0
	for _, stat := range stats {
		hitTotal += stat.hit
		notTotal += stat.notHit
		uncertainTotal += stat.uncertain
		unknownTotal += stat.unknown
		all += stat.total
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n样本 %d，重复 %d，阈值 %.2f/%.2f\n", len(first), repeat, hit, notHit)
	fmt.Fprintf(&b, "标签结果 %d：HIT %d，NOT_HIT %d，UNCERTAIN %d，UNKNOWN %d\n", all, hitTotal, notTotal, uncertainTotal, unknownTotal)
	fmt.Fprintf(&b, "可判定率 %s，UNKNOWN 率 %s\n", ratio(hitTotal+notTotal, all), ratio(unknownTotal, all))
	if gt := groundTruth(records, samples); len(gt) > 0 {
		var tp, fp, fn, tn int
		for _, g := range gt {
			tp += g.tp
			fp += g.fp
			fn += g.fn
			tn += g.tn
		}
		macroP, macroR := -1.0, -1.0
		if tp+fp > 0 {
			macroP = float64(tp) / float64(tp+fp)
		}
		if tp+fn > 0 {
			macroR = float64(tp) / float64(tp+fn)
		}
		fmt.Fprintf(&b, "真值(基线) 池化 Precision %s，Recall(已判) %s；TP %d FP %d FN %d TN %d\n",
			pctOrDash(macroP), pctOrDash(macroR), tp, fp, fn, tn)
	}
	groups := stabilityGroups(records)
	if len(groups) > 0 {
		flipped := 0
		for _, g := range groups {
			if g.buckets > 1 {
				flipped++
			}
		}
		fmt.Fprintf(&b, "跨桶翻转率 %s（%d/%d 组）\n", ratio(flipped, len(groups)), flipped, len(groups))
	}
	var elapsed []int64
	for _, rec := range records {
		elapsed = append(elapsed, rec.ElapsedMs)
	}
	fmt.Fprintf(&b, "耗时 P50 %d ms，P95 %d ms\n", percentile(elapsed, 0.5), percentile(elapsed, 0.95))
	return b.String()
}

func firstRound(records []record) []record {
	var out []record
	for _, rec := range records {
		if rec.Repeat == 1 {
			out = append(out, rec)
		}
	}
	return out
}

func ratio(part, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", float64(part)/float64(total)*100)
}

func joinCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		name := key
		if name == "" {
			name = "(空)"
		}
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[key]))
	}
	return strings.Join(parts, "，")
}
