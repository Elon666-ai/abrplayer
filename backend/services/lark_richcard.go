package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"backend/models"

	"gorm.io/gorm"
)

// ===========================================================
// 📦 基础结构定义
// ===========================================================

type LarkInteractiveCard struct {
	MsgType string       `json:"msg_type"`
	Card    LarkRichCard `json:"card"`
}

type LarkRichCard struct {
	Schema string      `json:"schema,omitempty"`
	Header *CardHeader `json:"header,omitempty"`
	Body   *CardBody   `json:"body,omitempty"`
}

type CardHeader struct {
	Template string       `json:"template,omitempty"`
	Title    *HeaderTitle `json:"title,omitempty"`
}

type HeaderTitle struct {
	Content string `json:"content,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

type CardBody struct {
	Elements []CardElement `json:"elements,omitempty"`
}

type CardElement struct {
	Tag         string            `json:"tag"`
	ElementID   string            `json:"element_id,omitempty"`
	Margin      string            `json:"margin,omitempty"`
	TextAlign   string            `json:"text_align,omitempty"`
	Content     string            `json:"content,omitempty"`
	HeaderStyle *TableHeaderStyle `json:"header_style,omitempty"`
	Columns     []TableColumn     `json:"columns,omitempty"`
	Rows        []map[string]any  `json:"rows,omitempty"`
	ChartSpec   *ChartSpec        `json:"chart_spec,omitempty"`
	Elements    []CardElement     `json:"elements,omitempty"` // 嵌套元素（div/fold）
}

// ===========================================================
// 📋 表格结构
// ===========================================================

type TableHeaderStyle struct {
	TextAlign string `json:"text_align,omitempty"`
	TextColor string `json:"text_color,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
}

type TableColumn struct {
	Name            string `json:"name,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	DataType        string `json:"data_type,omitempty"`
	HorizontalAlign string `json:"horizontal_align,omitempty"`
}

// ===========================================================
// 📊 图表结构
// ===========================================================

type ChartSpec struct {
	Type    string         `json:"type,omitempty"`
	Title   *ChartTitle    `json:"title,omitempty"`
	Data    []ChartDataset `json:"data,omitempty"`
	Series  []ChartSeries  `json:"series,omitempty"`
	Axes    []ChartAxis    `json:"axes,omitempty"`
	Legends *ChartLegends  `json:"legends,omitempty"`
}

type ChartTitle struct {
	Text string `json:"text,omitempty"`
}

type ChartDataset struct {
	Values []ChartDataPoint `json:"values,omitempty"`
}

type ChartDataPoint struct {
	X      string `json:"x"`
	Type   string `json:"type,omitempty"`
	Y      string `json:"y,omitempty"`
	YLabel string `json:"yLabel,omitempty"`
}

type ChartSeries struct {
	Type        string      `json:"type"`
	DataIndex   int         `json:"dataIndex"`
	Label       *ChartLabel `json:"label,omitempty"`
	SeriesField string      `json:"seriesField,omitempty"`
	XField      any         `json:"xField,omitempty"`
	YField      string      `json:"yField,omitempty"`
	// Style       any         `json:"style,omitempty"` // ✅ 新增 style 层
	Point any `json:"point,omitempty"`
}

type ChartLabel struct {
	Visible   bool   `json:"visible"`
	Formatter string `json:"formatter,omitempty"`
}

type ChartAxis struct {
	Orient      string          `json:"orient,omitempty"`
	SeriesIndex []int           `json:"seriesIndex,omitempty"`
	Label       *ChartAxisLabel `json:"label,omitempty"`
}

type ChartAxisLabel struct {
	Visible   bool   `json:"visible"`
	Formatter string `json:"formatter,omitempty"`
}

type ChartLegends struct {
	Visible bool   `json:"visible"`
	Orient  string `json:"orient,omitempty"`
}

// ===========================================================
// 🧠 构造卡片
// ===========================================================

type LarkCardOptions struct {
	Title     string
	HeaderTpl string
	Markdown  string
	TableCols []TableColumn
	TableRows []map[string]any
	Chart     *ChartSpec
	DetailURL string // “查看详情” 跳转地址
}

func BuildLarkInteractiveCard(opt LarkCardOptions) *LarkInteractiveCard {
	card := &LarkInteractiveCard{
		MsgType: "interactive",
		Card: LarkRichCard{
			Schema: "2.0",
			Header: &CardHeader{
				Template: opt.HeaderTpl,
				Title: &HeaderTitle{
					Content: opt.Title,
					Tag:     "plain_text",
				},
			},
			Body: &CardBody{},
		},
	}

	// Markdown
	if opt.Markdown != "" {
		card.Card.Body.Elements = append(card.Card.Body.Elements, CardElement{
			Tag:       "markdown",
			ElementID: "md_" + time.Now().Format("150405"),
			Margin:    "0px 20px 10px 20px",
			Content:   opt.Markdown,
			TextAlign: "left",
		})
	}

	// Table
	if len(opt.TableCols) > 0 {
		card.Card.Body.Elements = append(card.Card.Body.Elements, CardElement{
			Tag:       "table",
			ElementID: "tbl_" + time.Now().Format("150405"),
			Margin:    "0px 20px 10px 20px",
			HeaderStyle: &TableHeaderStyle{
				TextAlign: "center",
				// TextColor: "red",
				Bold: true,
			},
			Columns: opt.TableCols,
			Rows:    opt.TableRows,
		})
	}

	// Chart（可折叠），不支持fold

	// Chart（直接附加，无fold）
	if opt.Chart != nil {
		card.Card.Body.Elements = append(card.Card.Body.Elements, CardElement{
			Tag:       "chart",
			ChartSpec: opt.Chart,
		})
	}

	// 查看详情
	if opt.DetailURL != "" {
		card.Card.Body.Elements = append(card.Card.Body.Elements, CardElement{
			Tag:       "markdown",
			Content:   fmt.Sprintf("📈 **[View Details](%s)**", opt.DetailURL),
			TextAlign: "left",
			Margin:    "10px 20px 10px 20px",
		})
	}

	return card
}

// ===========================================================
// 📊 BuildChart
// ===========================================================

type ChartData struct {
	SeriesName string
	Type       string
	Points     []ChartDataPoint
}

func BuildChart(title, leftLabelFmt string, dataSets []ChartData) *ChartSpec {
	spec := &ChartSpec{
		Type: "common",
		Title: &ChartTitle{
			Text: title,
		},
	}

	for _, ds := range dataSets {
		spec.Data = append(spec.Data, ChartDataset{Values: ds.Points})
	}

	for i, ds := range dataSets {
		s := ChartSeries{
			Type:        ds.Type,
			DataIndex:   i,
			SeriesField: "type",
			YField:      "y",
		}
		if ds.Type == "bar" {
			s.XField = []string{"x", "type"}
			s.Label = &ChartLabel{Visible: false, Formatter: "{yLabel}"}
		} else {
			s.XField = "x"
			s.Label = &ChartLabel{Visible: false}
			// ✅ 隐藏折线图上的点
			s.Point = map[string]any{
				"visible": false,
				"size":    0,
				"shape":   "none",
				"style": map[string]any{
					"r":       0, // radius 0
					"opacity": 0, // fully transparent
				},
			}

		}
		spec.Series = append(spec.Series, s)
	}

	spec.Axes = []ChartAxis{
		{Orient: "left", SeriesIndex: []int{0, 1}, Label: &ChartAxisLabel{Visible: true, Formatter: leftLabelFmt}},
		{Orient: "right", SeriesIndex: []int{2}, Label: &ChartAxisLabel{Visible: true}},
		{Orient: "bottom", Label: &ChartAxisLabel{Visible: true}},
	}

	spec.Legends = &ChartLegends{Visible: true, Orient: "bottom"}
	return spec
}

// ===========================================================
// 📋 BuildTable
// ===========================================================

func BuildTable(headers []string, data [][]string, align, color string) ([]TableColumn, []map[string]any) {
	cols := make([]TableColumn, len(headers))
	for i, h := range headers {
		name := fmt.Sprintf("col_%d", i)
		cols[i] = TableColumn{Name: name, DisplayName: h, DataType: "lark_md", HorizontalAlign: align}
	}

	rows := make([]map[string]any, len(data))
	for i, row := range data {
		m := map[string]any{}
		for j, cell := range row {
			if j < len(cols) {
				name := cols[j].Name
				m[name] = fmt.Sprintf("<font color='%s'>%s</font>", color, cell)
			}
		}
		rows[i] = m
	}
	return cols, rows
}

// ===========================================================
// 🚀 SendToLarkWebhook
//2025-11-04 16:26:57.535407 [INFO][  APP][00367][stats.go:164]RoundReport send to lark failure!
// send failed: Post "V6946UJXEyqwrDVBlyPW7d": unsupported protocol scheme ""
// ===========================================================

func SendToLarkWebhook(webhookURL, secret string, card *LarkInteractiveCard) error {
	timestamp := time.Now().Unix()

	payload := map[string]any{
		"msg_type": card.MsgType,
		"card":     card.Card,
	}

	if secret != "" {
		sign := genSign(secret, timestamp)
		payload["timestamp"] = timestamp
		payload["sign"] = sign
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// BuildInteractiveChart 从数据库中统计播放请求、成功率与平均成功率
// 支持 interval: "minute", "hour", "day", "5m", "10m", "15m"
func BuildInteractiveChart(db *gorm.DB, startTime, stopTime time.Time, interval string, streamPath, roundId string) (
	ChartData, ChartData, ChartData, error) {
	// 参数检查
	if stopTime.Before(startTime) {
		return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("stopTime < startTime")
	}

	interval = normalizeInterval(interval)
	step := getStep(interval)

	// 查询 statstartplay（models.StartPlayStat）
	var plays []models.StartPlayStat
	if roundId == "" {
		if streamPath == "" {
			if err := db.Select("isSucceed, createdAt").
				Where("createdAt >= ? AND createdAt <= ?", startTime, stopTime).
				Find(&plays).Error; err != nil {
				return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("query StartPlayStat failed: %w", err)
			}
		} else {
			if err := db.Select("isSucceed, createdAt").
				Where("createdAt >= ? AND createdAt <= ? AND streamName = ?", startTime, stopTime, streamPath).
				Find(&plays).Error; err != nil {
				return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("query StartPlayStat failed: %w", err)
			}
		}
	} else {
		if streamPath == "" {
			if err := db.Select("isSucceed, createdAt").
				Where("createdAt >= ? AND createdAt <= ? AND gameRound = ?", startTime, stopTime, roundId).
				Find(&plays).Error; err != nil {
				return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("query StartPlayStat failed: %w", err)
			}
		} else {
			if err := db.Select("isSucceed, createdAt").
				Where("createdAt >= ? AND createdAt <= ? AND streamName = ? AND gameRound = ?", startTime, stopTime, streamPath, roundId).
				Find(&plays).Error; err != nil {
				return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("query StartPlayStat failed: %w", err)
			}
		}
	}

	if len(plays) == 0 {
		return ChartData{}, ChartData{}, ChartData{}, fmt.Errorf("no data in range %s ~ %s",
			startTime.Format(time.RFC3339), stopTime.Format(time.RFC3339))
	}

	// 聚合桶
	type Bucket struct {
		RequestCount int
		SuccessCount int
	}
	buckets := make(map[time.Time]*Bucket)
	var totalReq, totalSucc int

	for _, p := range plays {
		key := truncateByInterval(p.CreatedAt, step)
		b, ok := buckets[key]
		if !ok {
			b = &Bucket{}
			buckets[key] = b
		}
		b.RequestCount++
		if p.IsSucceed {
			b.SuccessCount++
		}
		totalReq++
		if p.IsSucceed {
			totalSucc++
		}
	}

	// 排序时间点
	times := make([]time.Time, 0, len(buckets))
	for t := range buckets {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	// 按步长补全（如果数据不多）
	const maxPoints = 7 * 24 * 60 // 最多7天分钟粒度
	fullSeries := make([]time.Time, 0)
	count := int(stopTime.Sub(startTime) / step)
	if count <= maxPoints {
		for t := truncateByInterval(startTime, step); !t.After(stopTime); t = t.Add(step) {
			fullSeries = append(fullSeries, t)
			if _, ok := buckets[t]; !ok {
				buckets[t] = &Bucket{}
			}
		}
	} else {
		fullSeries = times
	}

	// 格式化时间标签
	xFormat := formatForInterval(interval)

	// 生成ChartData
	playRequests := ChartData{SeriesName: "playRequests", Type: "bar"}
	playSuccessRate := ChartData{SeriesName: "playSuccessRate", Type: "line"}
	avgPlaySuccessRate := ChartData{SeriesName: "avgPlaySuccessRate", Type: "line"}

	avgRate := 0.0
	if totalReq > 0 {
		avgRate = float64(totalSucc) * 100.0 / float64(totalReq)
	}

	for _, t := range fullSeries {
		b := buckets[t]
		rate := 0.0
		if b.RequestCount > 0 {
			rate = float64(b.SuccessCount) * 100.0 / float64(b.RequestCount)
		}

		label := t.Format(xFormat)

		playRequests.Points = append(playRequests.Points, ChartDataPoint{
			X:      label,
			Type:   "playRequests",
			Y:      fmt.Sprintf("%d", b.RequestCount),
			YLabel: fmt.Sprintf("%d", b.RequestCount),
		})

		playSuccessRate.Points = append(playSuccessRate.Points, ChartDataPoint{
			X:    label,
			Type: "playSuccessRate",
			Y:    fmt.Sprintf("%.2f", rate),
		})

		avgPlaySuccessRate.Points = append(avgPlaySuccessRate.Points, ChartDataPoint{
			X:    label,
			Type: "avgPlaySuccessRate",
			Y:    fmt.Sprintf("%.2f", avgRate),
		})
	}

	return playRequests, playSuccessRate, avgPlaySuccessRate, nil
}

// normalizeInterval 支持 minute/hour/day/5m/10m/15m
func normalizeInterval(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	switch in {
	case "hour", "h", "1h":
		return "hour"
	case "day", "d", "1d":
		return "day"
	case "5m", "5min", "5minute":
		return "5m"
	case "10m", "10min", "10minute":
		return "10m"
	case "15m", "15min", "15minute":
		return "15m"
	default:
		return "minute"
	}
}

// getStep 根据 interval 返回 time.Duration
func getStep(interval string) time.Duration {
	switch interval {
	case "hour":
		return time.Hour
	case "day":
		return 24 * time.Hour
	case "5m":
		return 5 * time.Minute
	case "10m":
		return 10 * time.Minute
	case "15m":
		return 15 * time.Minute
	default:
		return time.Minute
	}
}

// truncateByInterval 按指定步长截断时间
func truncateByInterval(t time.Time, step time.Duration) time.Time {
	// 按天特殊处理
	if step >= 24*time.Hour {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	}
	ns := t.UnixNano()
	stepNs := int64(step)
	return time.Unix(0, ns-ns%stepNs).In(t.Location())
}

// formatForInterval 决定 X 轴格式
func formatForInterval(interval string) string {
	switch interval {
	case "day":
		// return "2006-01-02"
		return "01-02"
	case "hour":
		return "15"
		// return "15:00"
		// return "01-02 15:00"
	case "5m", "10m", "15m", "minute":
		return "15:04"
		// return "01-02 15:04"
	default:
		return "2006-01-02 15:04"
	}
}
