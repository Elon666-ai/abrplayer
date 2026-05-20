package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"backend/models"
	"backend/utils"
)

// =======================================
// 统一类型定义
// =======================================

// 消息类型
type LarkMessageType string

const (
	TextMsg     LarkMessageType = "text"
	MarkdownMsg LarkMessageType = "post"        // 富文本消息
	CardMsg     LarkMessageType = "interactive" // 卡片消息
)

// ---------- 1️⃣ 文本消息 ----------
type LarkTextContent struct {
	Text string `json:"text"`
}

// ---------- 2️⃣ 富文本消息 ----------
type LarkMarkdownContent struct {
	Post struct {
		ZhCn struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag  string `json:"tag"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"zh_CN"`
	} `json:"post"`
}

// ---------- 3️⃣ 卡片消息 ----------
type LarkCard struct {
	Config struct {
		WideScreenMode bool `json:"wide_screen_mode"`
		EnableForward  bool `json:"enable_forward"`
	} `json:"config"`
	Header struct {
		Title struct {
			Tag     string `json:"tag"`
			Content string `json:"content"`
		} `json:"title"`
		Template string `json:"template"`
	} `json:"header"`
	Elements []interface{} `json:"elements"`
}

// ---------- 统一封装 ----------
type LarkMessage struct {
	Timestamp int64           `json:"timestamp"`
	Sign      string          `json:"sign"`
	MsgType   LarkMessageType `json:"msg_type"`
	Content   interface{}     `json:"content,omitempty"`
	Card      *LarkCard       `json:"card,omitempty"`
}

// =======================================
// 签名函数（Lark v3 标准）
// =======================================
func genSign(secret string, ts int64) string {
	stringToSign := fmt.Sprintf("%v\n%s", ts, secret)
	h := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// =======================================
// ✅ Lark Response: 200 {"StatusCode":0,"StatusMessage":"success","code":0,"data":{},"msg":"success"}
// =======================================
func postToLark(webhook string, msg LarkMessage) error {
	body, _ := json.Marshal(msg)
	req, err := http.NewRequest("POST", webhook, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// b, _ := io.ReadAll(resp.Body)
	// log.Printf("✅ Lark Response: %d %s", resp.StatusCode, string(b))
	return nil
}

// =======================================
// 各类消息构造函数
// =======================================

// 1️⃣ Text
func buildTextMessage(secret, text string) LarkMessage {
	ts := time.Now().Unix()
	return LarkMessage{
		Timestamp: ts,
		Sign:      genSign(secret, ts),
		MsgType:   TextMsg,
		Content:   LarkTextContent{Text: text},
	}
}

// 2️⃣ Markdown
func buildMarkdownMessage(secret string, r RoundReport) LarkMessage {
	ts := time.Now().Unix()

	durationSec := r.FinishTime.Sub(r.StartTime).Seconds()
	statusIcon := "❌"
	if r.StreamingStatus {
		statusIcon = "✅"
	}
	var md LarkMarkdownContent
	md.Post.ZhCn.Title = fmt.Sprintf("Round ID:  %s", r.RoundID)
	md.Post.ZhCn.Content = [][]struct {
		Tag  string `json:"tag"`
		Text string `json:"text"`
	}{
		{
			{Tag: "text", Text: fmt.Sprintf("Duration: %.0f s\n", durationSec)},
			{Tag: "text", Text: fmt.Sprintf("Streaming Status:       %s\n", statusIcon)},
			{Tag: "text", Text: fmt.Sprintf("Stream Requests:        %d\n", r.StreamRequests)},
			{Tag: "text", Text: fmt.Sprintf("Max Retry Count:        %d\n", r.MaxRetryCount)},
			{Tag: "text", Text: fmt.Sprintf("Recording Completeness: %.1f%%\n", r.VideoCompleteness)},
			{Tag: "text", Text: fmt.Sprintf("Play Success Rate:      %.1f%%\n", r.PlaySuccessRate)},
			{Tag: "text", Text: fmt.Sprintf("Avg Startup Time:       %.1f s", r.AverageStartupTime)},
		},
	}

	return LarkMessage{
		Timestamp: ts,
		Sign:      genSign(secret, ts),
		MsgType:   MarkdownMsg,
		Content:   md,
	}
}

// 3️⃣ Card
func buildCardMessage(secret string, r MetricCardMsg, statPeriod int) LarkMessage {
	ts := time.Now().Unix()
	card := &LarkCard{}

	card.Config.WideScreenMode = true
	card.Config.EnableForward = true
	card.Header.Title.Tag = "plain_text"
	card.Header.Title.Content = fmt.Sprintf("%s Monitoring Report", r.FieldName)
	card.Header.Template = "turquoise"

	var statDate string = r.StatHour.Format("📅 **2006-01-02 15:04:05**")
	if statPeriod == 1 {
		statDate = r.StatHour.Format("**🕒 2006-01-02**")
	}
	card.Elements = []interface{}{
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": statDate,
			},
		},
		map[string]interface{}{
			"tag": "column_set",
			"columns": []map[string]interface{}{
				{
					"tag": "column",
					"elements": []map[string]interface{}{
						{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": "Metric\nPlay Success Rate\nRecording Success Rate\nStream Requests\nAverage Startup Time\nStream Break Count"}},
					},
				},
				{
					"tag": "column",
					"elements": []map[string]interface{}{
						{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": fmt.Sprintf("Current Value\n%.2f%%\n%.2f%%\n%d\n%.1f s\n%d",
							r.AvgStreamSuccessRate, r.AvgRecordSuccessRate,
							r.RequestStreamTotal, float64(r.AvgLoadTime)/1000, r.ReconnectCount)}},
					},
				},
				{
					"tag": "column",
					"elements": []map[string]interface{}{
						{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": "Normal Range\n≥95%\n≥95%\n\n≤2 s\n≤10"}},
					},
				},
				{
					"tag": "column",
					"elements": []map[string]interface{}{
						{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": fmt.Sprintf("Status\n%s\n%s\n%s\n%s\n%s", statusEmoji(r.AvgStreamSuccessRate >= 0.95),
							statusEmoji(r.AvgRecordSuccessRate >= 0.95), statusEmoji(r.RequestStreamTotal > 0),
							statusEmoji(r.AvgLoadTime <= 2000), statusEmoji(r.ReconnectCount <= 10))}},
					},
				},
			},
		},
		map[string]interface{}{
			"tag": "action",
			"actions": []map[string]interface{}{
				{
					"tag":  "button",
					"text": map[string]string{"tag": "plain_text", "content": "查看详情"},
					"url":  "https://www.example.com/report",
					"type": "primary",
				},
			},
		},
	}

	return LarkMessage{
		Timestamp: ts,
		Sign:      genSign(secret, ts),
		MsgType:   CardMsg,
		Card:      card,
	}
}

func TriggerAlarmToLark(trigger string, info AlertInfo) {
	now := time.Now().Format("2006-01-02 15:04:05")
	alarmText := ""

	switch trigger {
	case utils.AlarmStreamBroken:
		if info.DisconnectionDuration > 0 {
			alarmText = fmt.Sprintf(
				`%s (Alert time)
⚠️%s Incident
- [%s]Streaming Status: ❌
        • Disconnection Duration: %v
`,
				now,
				info.FieldName,
				info.StreamPath,
				info.DisconnectionDuration,
			)
		} else {
			alarmText = fmt.Sprintf(
				`%s (Alert time)
⚠️%s Incident
- [%s]Streaming Status: ❌
`,
				now,
				info.FieldName,
				info.StreamPath,
			)
		}
	case utils.AlarmPlayTimeout:
		alarmText = fmt.Sprintf(
			`%s (Alert time)
⚠️%s Incident
- [%s]Streaming heavy delay: %d s
`,
			now,
			info.FieldName,
			info.StreamPath,
			info.StreamPullHeavyDelay/1000,
		)
	case utils.AlarmRecordNotComplete:
		alarmText = fmt.Sprintf(
			`%s (Alert time)
⚠️Incident
- Video Recording Completeness: %d%%⬇️
        • Round ID：%s
		• Round Duration：[%s, %s]
        • Recording Duration : %v
        • URL: %s
`,
			now,
			info.RecordCompleteness,
			info.RoundID,
			info.RoundStartTime.Format("2006-01-02 15:04:05"),
			info.RoundEndTime.Format("2006-01-02 15:04:05"),
			info.RecordingDuration,
			info.RecordingURL,
		)
	default:

	}

	textMsg := buildTextMessage(models.GetLarkSecret(), alarmText)
	postToLark(models.GetLarkWebhook(), textMsg)
}

// StatusMark —— 返回状态图标
func statusEmoji(ok bool) string {
	if ok {
		return "✅"
	}
	return "⚠️"
}

func PostTextToLark(text string) error {
	msg := buildTextMessage(models.GetLarkSecret(), text)
	return postToLark(models.GetLarkWebhook(), msg)
}
