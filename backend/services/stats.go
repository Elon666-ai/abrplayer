package services

import (
	"fmt"
	"math"
	"time"
	"backend/models"
	"backend/tracer"
	"backend/utils"

	"gorm.io/gorm"
)

// AlertInfo 定义告警数据结构
type AlertInfo struct {
	FieldName             string        //现场名称
	StreamPath            string        //现场流名称
	DisconnectionDuration time.Duration // 中断时长
	StreamPullHeavyDelay  int           //拉流时延
	RecordCompleteness    int           // 录像完整度百分比
	RoundID               string        // 轮次ID
	RoundStartTime        time.Time     // 轮次开始时间
	RoundEndTime          time.Time     // 轮次结束时间
	RecordingDuration     time.Duration // 实际录像时长
	RecordingURL          string        // 录像URL
}

type RoundReport struct {
	RoundID            string
	StartTime          time.Time
	FinishTime         time.Time
	StreamingStatus    bool
	StreamRequests     int
	MaxRetryCount      int
	VideoCompleteness  float64
	PlaySuccessRate    float64
	AverageStartupTime float64
}

type MetricCardMsg struct {
	FieldName            string
	StatHour             time.Time
	CreatedAt            time.Time
	AvgStreamSuccessRate float64
	AvgRecordSuccessRate float64
	RequestStreamTotal   int64
	AvgLoadTime          int64
	ReconnectCount       int64
}

func AggregateRoundStat(roundId string, endTime *time.Time) error {
	db := models.GetDbConn()

	var count int64 = 0
	db.Where(&models.RoundVideoStat{RoundId: roundId}).Count(&count)
	if count >= 1 {
		return nil // 已经统计过了
	}

	var round models.FieldRoundStat
	if err := db.Where("roundId = ?", roundId).First(&round).Error; err != nil {
		tracer.LogDebug(tracer.ID_APP, "no such round:%s, %v", roundId, err)
		return err
	}

	var record models.RecordStat
	if err := db.Where("gameRound LIKE ?", fmt.Sprintf("%%%s%%", roundId)).First(&record).Error; err != nil {
		tracer.LogDebug(tracer.ID_APP, "[%s]round not recordings!", roundId)
	}

	start := round.RoundStartTime
	var end time.Time = start
	if endTime != nil {
		end = *endTime
	} else {
		if round.RoundEndTime != nil {
			end = *round.RoundEndTime
		}
	}
	// 安全检查：若 end 早于 start，则把 end 设为 start+1min
	if end.IsZero() || end.Before(start) {
		end = start.Add(time.Minute)
	}
	var stats struct {
		RequestStreamTotal  int64
		StreamSuccessNum    int64
		ReconnectCount      int64
		AvgLoadTime         float64
		RecordDurationTotal int64
	}

	// 统计拉流信息
	pattern := fmt.Sprintf("%%%s%%", roundId)

	db.Raw(`
    SELECT 
        COUNT(*) AS RequestStreamTotal,
        SUM(CASE WHEN isSucceed=1 THEN 1 ELSE 0 END) AS StreamSuccessNum,
        SUM(CASE WHEN intervalFromLastPlay<10000 AND intervalFromLastPlay>1 THEN 1 ELSE 0 END) AS ReconnectCount,
        AVG(waitMiliTime) AS AvgLoadTime
    FROM statstartplay 
    WHERE gameRound LIKE ?`, pattern).Scan(&stats)

	// 录像总时长
	db.Raw(`SELECT IFNULL(SUM(recordDuration),0) AS RecordDurationTotal FROM statrecord WHERE gameRound LIKE ?`, fmt.Sprintf("%%%s%%", roundId)).Scan(&stats.RecordDurationTotal)

	liveDuration := int64(end.Sub(start).Seconds())

	videoCompletionRate := 0.0
	if liveDuration > 0 {
		videoCompletionRate = float64(stats.RecordDurationTotal) / float64(liveDuration) * 100
		// 录像时长占比不到20%就告警!
		if videoCompletionRate < 20 {
			tracer.LogWarn(tracer.ID_APP, "[RoundStat:%s] RecordDurationTotal/liveDuration = %d/%d %%", roundId, stats.RecordDurationTotal*100, liveDuration)
			go TriggerAlarmToLark(utils.AlarmRecordNotComplete,
				AlertInfo{RecordCompleteness: int(videoCompletionRate), RoundID: roundId,
					RoundStartTime: start, RoundEndTime: end,
					RecordingDuration: time.Duration(stats.RecordDurationTotal) * time.Second,
					RecordingURL:      record.PlaybackUri})
		}
	}

	streamSuccessRate := 0.0
	if stats.RequestStreamTotal > 0 {
		streamSuccessRate = float64(stats.StreamSuccessNum) / float64(stats.RequestStreamTotal) * 100
	}

	result := models.RoundVideoStat{
		RoundId:             roundId,
		FieldName:           round.FieldName,
		RoundStartTime:      start,
		RoundEndTime:        end,
		LiveDuration:        liveDuration,
		RequestStreamTotal:  stats.RequestStreamTotal,
		StreamSuccessNum:    stats.StreamSuccessNum,
		ReconnectCount:      stats.ReconnectCount,
		AvgLoadTime:         stats.AvgLoadTime,
		RecordDurationTotal: stats.RecordDurationTotal,
		VideoCompletionRate: videoCompletionRate,
		StreamSuccessRate:   streamSuccessRate,
	}

	err := db.Create(&result).Error
	if err != nil {
		tracer.LogWarn(tracer.ID_APP, "[RoundStat] insert failed: %v", err)
	} else {
		tracer.LogDebug(tracer.ID_APP, "[RoundStat] inserted OK for %s", roundId)
	}

	// textMsg := buildMarkdownMessage(models.GetLarkSecret(), RoundReport{
	// 	RoundID:            roundId,
	// 	StartTime:          start,
	// 	FinishTime:         end,
	// 	StreamingStatus:    true,
	// 	StreamRequests:     int(stats.StreamSuccessNum),
	// 	MaxRetryCount:      int(stats.ReconnectCount),
	// 	VideoCompleteness:  videoCompletionRate,
	// 	PlaySuccessRate:    streamSuccessRate,
	// 	AverageStartupTime: stats.AvgLoadTime,
	// })
	// err = postToLark(models.GetLarkWebhook(), textMsg)
	// if err != nil {
	// 	tracer.LogInfo(tracer.ID_APP, "RoundReport send to lark failure! %v", err)
	// }

	var chart *ChartSpec = nil
	playReq, playSucc, avgSucc, err := BuildInteractiveChart(db, start, end, "minute", "", roundId)
	if err != nil {
		tracer.LogDebug(tracer.ID_APP, "build RoundReport failure! %v", err)
	} else {
		chart = BuildChart("", "{label}%", []ChartData{playSucc, avgSucc, playReq})
	}

	card := BuildLarkInteractiveCard(LarkCardOptions{
		Title:     "Monitoring Report (Per-Round)",
		HeaderTpl: "blue",
		Markdown: fmt.Sprintf("**Round ID: %s**\n%s(start time)-%s(finish time)\n"+
			"Streaming Status %s\nStream Requests %d\nMax Retry Count %d\nVideo Recording Completeness %d%%\n"+
			"Play Success Rate %d%%\nAverage Startup Time %d s\n",
			roundId, start.Format("15:04:05"), end.Format("15:04:05"),
			statusEmoji(true), int(stats.StreamSuccessNum), int(stats.ReconnectCount),
			int(videoCompletionRate), int(streamSuccessRate), int(math.Round(stats.AvgLoadTime))),
		Chart:     chart,
		DetailURL: utils.GetStatDetailUrl(),
	})

	if err = SendToLarkWebhook(models.GetLarkWebhook(), models.GetLarkSecret(), card); err != nil {
		tracer.LogWarn(tracer.ID_APP, "RoundReport send to lark failure! %v", err)
	}

	tracer.LogInfo(tracer.ID_APP, "[%s]RoundReport send to lark success!", roundId)
	return err
}

func AggregateHourlyStat(db *gorm.DB) {

	for field, streams := range FieldStreamMap {
		for _, uri := range streams {
			streamPath := utils.ExtractStreamPath(uri)
			now := time.Now()
			// startHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
			startHour := now.Add(-1 * time.Hour)
			endHour := now

			type Temp struct {
				RequestStreamTotal int64
				StreamSuccessNum   int64
				AvgLoadTime        float64
				LagDurationTotal   int64
				PlayDurationTotal  int64
				LagCount           int64
				RecordTotal        int64
				RecordSuccessNum   int64
				PlaybackTotal      int64
				PlaybackSuccessNum int64
				ReconnectCount     int64
			}

			var tmp Temp
			err := db.Raw(`
SELECT
  (SELECT COUNT(*) FROM statstartplay WHERE createdAt BETWEEN ? AND ? AND streamName=?) AS RequestStreamTotal,
  (SELECT COUNT(*) FROM statstartplay WHERE isSucceed=1 AND createdAt BETWEEN ? AND ? AND streamName=?) AS StreamSuccessNum,
  (SELECT IFNULL(AVG(waitMiliTime),0) FROM statstartplay WHERE createdAt BETWEEN ? AND ? AND streamName=?) AS AvgLoadTime,
  (SELECT IFNULL(SUM(lagDuration),0) FROM statplaylag WHERE createdAt BETWEEN ? AND ? AND streamName=?) AS LagDurationTotal,
  (SELECT IFNULL(SUM(playDuration),0) FROM statendplay WHERE createdAt BETWEEN ? AND ? AND streamName=?) AS PlayDurationTotal,
  (SELECT COUNT(*) FROM statplaylag WHERE lagDuration>2000 AND createdAt BETWEEN ? AND ? AND streamName=?) AS LagCount,
  (SELECT COUNT(*) FROM statstartplay WHERE intervalFromLastPlay<10000 AND intervalFromLastPlay>1000 AND createdAt BETWEEN ? AND ? AND streamName=?) AS ReconnectCount,
  (SELECT COUNT(*) FROM statrecord WHERE createdAt BETWEEN ? AND ?) AS RecordTotal,
  (SELECT COUNT(*) FROM statrecord WHERE isSucceed=1 AND createdAt BETWEEN ? AND ?) AS RecordSuccessNum,
  (SELECT COUNT(*) FROM statplayback WHERE createdAt BETWEEN ? AND ?) AS PlaybackTotal,
  (SELECT COUNT(*) FROM statplayback WHERE isSucceed=1 AND createdAt BETWEEN ? AND ?) AS PlaybackSuccessNum
`,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour, streamPath,
				startHour, endHour,
				startHour, endHour,
				startHour, endHour,
				startHour, endHour,
			).Scan(&tmp).Error
			if err != nil {
				tracer.LogWarn(tracer.ID_APP, "AggregateHourlyStat error: %v", err)
				continue
			}

			startHour = startHour.Add(time.Second)
			result := models.HourlyVideoStat{
				FieldName:          field,
				StreamPath:         streamPath,
				StatHour:           startHour,
				ReconnectCount:     tmp.ReconnectCount,
				RequestStreamTotal: tmp.RequestStreamTotal,
				StreamSuccessNum:   tmp.StreamSuccessNum,
				AvgLoadTime:        tmp.AvgLoadTime,
				LagDurationTotal:   tmp.LagDurationTotal,
				PlayDurationTotal:  tmp.PlayDurationTotal,
				LagCount:           tmp.LagCount,
				RecordTotal:        tmp.RecordTotal,
				RecordSuccessNum:   tmp.RecordSuccessNum,
				PlaybackTotal:      tmp.PlaybackTotal,
				PlaybackSuccessNum: tmp.PlaybackSuccessNum,
				CreatedAt:          time.Now(),
			}

			if tmp.RequestStreamTotal == 0 {
				tracer.LogWarn(tracer.ID_APP, "%s no pull request, skip to process!", field)
				continue
			}
			ss, _ := GetStreamState(streamPath)
			if ss != utils.StateActive {
				tracer.LogWarn(tracer.ID_APP, "%s:%s not active, skip to process! %s", field, streamPath, ss)
				continue
			}

			// 派生计算
			if tmp.RequestStreamTotal > 0 {
				result.AvgStreamSuccessRate = float64(tmp.StreamSuccessNum) / float64(tmp.RequestStreamTotal) * 100
			}
			if tmp.PlayDurationTotal > 0 {
				result.AvgLagRate = float64(tmp.LagDurationTotal) / float64(tmp.PlayDurationTotal) * 1.0
			}
			if tmp.StreamSuccessNum > 0 {
				result.AvgPlayDuration = float64(tmp.PlayDurationTotal) / float64(tmp.StreamSuccessNum) / 1000.0
			}
			if tmp.RecordTotal > 0 {
				result.AvgRecordSuccessRate = float64(tmp.RecordSuccessNum) / float64(tmp.RecordTotal) * 100
			}
			if tmp.PlaybackTotal > 0 {
				result.AvgPlaybackSuccessRate = float64(tmp.PlaybackSuccessNum) / float64(tmp.PlaybackTotal) * 100
			}

			if err := db.Create(&result).Error; err != nil {
				tracer.LogWarn(tracer.ID_APP, "insert hourly stat failed: %v", err)
			} else {
				tracer.LogInfo(tracer.ID_APP, "[HourlyStat] %s, section: %s--%s", field, startHour.Format("2006-01-02 15:00:00"), endHour.Format("2006-01-02 15:00:00"))
			}

			if result.AvgLoadTime > 4000 {
				go TriggerAlarmToLark(utils.AlarmPlayTimeout,
					AlertInfo{FieldName: field, StreamPath: result.StreamPath, StreamPullHeavyDelay: int(result.AvgLoadTime)})
			}

			// if result.RequestStreamTotal > 0 {
			// 	stat := MetricCardMsg{}
			// 	stat.FieldName = field
			// 	stat.StatHour = result.StatHour
			// 	stat.CreatedAt = result.CreatedAt
			// 	stat.AvgStreamSuccessRate = result.AvgStreamSuccessRate
			// 	stat.AvgRecordSuccessRate = result.AvgRecordSuccessRate
			// 	stat.RequestStreamTotal = result.RequestStreamTotal
			// 	stat.AvgLoadTime = int64(result.AvgLoadTime)
			// 	stat.ReconnectCount = result.ReconnectCount
			// 	textMsg := buildCardMessage(models.GetLarkSecret(), stat, 0)
			// 	err = postToLark(models.GetLarkWebhook(), textMsg)
			// 	if err != nil {
			// 		tracer.LogInfo(tracer.ID_APP, "HourlyReport send to lark failure! %v", err)
			// 	}
			// }

			var chart *ChartSpec = nil
			var statDate string = startHour.Format("📅 **2006-01-02 15:04:05**") + endHour.Format(" - **15:04:05**")
			end := time.Now()
			start := end.Add(time.Duration(-23 * time.Hour)) // 最近24小时趋势
			playReq, playSucc, avgSucc, err := BuildInteractiveChart(db, start, end, "hour", streamPath, "")
			if err != nil {
				tracer.LogInfo(tracer.ID_APP, "build HourlyReport failure! %v", err)
			} else {
				chart = BuildChart("PlaySuccessRate Trend", "{label}%", []ChartData{playSucc, avgSucc, playReq})
			}
			cardOps := LarkCardOptions{
				Title:     fmt.Sprintf("Hourly Monitoring (%s)", field),
				HeaderTpl: "blue",
				Markdown:  statDate,
				Chart:     chart,
				DetailURL: utils.GetStatDetailUrl(),
			}
			if result.RequestStreamTotal > 0 {
				cols := []TableColumn{
					{Name: "metric", DisplayName: "Metric", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "value", DisplayName: "Current Value", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "range", DisplayName: "Normal Range", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "status", DisplayName: "Status", DataType: "lark_md", HorizontalAlign: "center"},
				}
				rows := []map[string]any{
					{"metric": "Play Success Rate", "value": fmt.Sprintf("%.2f%%", result.AvgStreamSuccessRate), "range": "≥95%", "status": statusEmoji(result.AvgStreamSuccessRate >= 95)},
					{"metric": "Recording Success Rate", "value": fmt.Sprintf("%.2f%%", result.AvgRecordSuccessRate), "range": "≥95%", "status": statusEmoji(result.AvgRecordSuccessRate >= 95)},
					{"metric": "Play Requests", "value": fmt.Sprintf("%d", result.RequestStreamTotal), "range": "", "status": statusEmoji(result.RequestStreamTotal > 0)},
					{"metric": "Average Startup Time", "value": fmt.Sprintf("%.1f s", result.AvgLoadTime/1000), "range": "≤2 s", "status": statusEmoji(result.AvgLoadTime <= 2000)},
					{"metric": "Play Break Count", "value": fmt.Sprintf("%d", result.ReconnectCount), "range": "≤10", "status": statusEmoji(result.ReconnectCount <= 10)},
				}
				cardOps.TableCols = cols
				cardOps.TableRows = rows
			}
			card := BuildLarkInteractiveCard(cardOps)

			if err = SendToLarkWebhook(models.GetLarkWebhook(), models.GetLarkSecret(), card); err != nil {
				tracer.LogInfo(tracer.ID_APP, "HourlyReport send to lark failure! %v", err)
			} else {
				tracer.LogInfo(tracer.ID_APP, "HourlyReport send to lark success!")
			}
		}
	}
}

func AggregateDailyStat(db *gorm.DB) {
	for field, streams := range FieldStreamMap {
		for _, uri := range streams {
			streamPath := utils.ExtractStreamPath(uri)
			var result models.DailyVideoStat

			// 计算昨日时间范围（使用本地时区）
			now := time.Now()
			yesterday := now.AddDate(0, 0, -1)
			startTime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, now.Location())
			endTime := startTime.Add(24 * time.Hour)

			type Temp struct {
				StreamTotal        int64
				StreamSuccessNum   int64
				PlayDurationTotal  int64
				LagDurationTotal   int64
				LagCount           int64
				RecordTotal        int64
				RecordSuccessNum   int64
				PlaybackTotal      int64
				PlaybackSuccessNum int64
			}

			var tmp Temp
			// 使用时间区间查询（避免对索引列使用函数 DATE(...) 导致索引失效）
			if err := db.Raw(`
SELECT 
  (SELECT COUNT(*) FROM statstartplay WHERE createdAt >= ? AND createdAt < ? AND streamName=?) AS StreamTotal,
  (SELECT COUNT(*) FROM statstartplay WHERE isSucceed=1 AND createdAt >= ? AND createdAt < ? AND streamName=?) AS StreamSuccessNum,
  (SELECT IFNULL(AVG(waitMiliTime),0) FROM statstartplay WHERE createdAt BETWEEN ? AND ? AND streamName=?) AS AvgLoadTime,
  (SELECT IFNULL(SUM(playDuration),0) FROM statendplay WHERE createdAt >= ? AND createdAt < ? AND streamName=?) AS PlayDurationTotal,
  (SELECT IFNULL(SUM(lagDuration),0) FROM statplaylag WHERE createdAt >= ? AND createdAt < ? AND streamName=?) AS LagDurationTotal,
  (SELECT COUNT(*) FROM statplaylag WHERE lagDuration > 3000 AND createdAt >= ? AND createdAt < ? AND streamName=?) AS LagCount,
  (SELECT COUNT(*) FROM statstartplay WHERE intervalFromLastPlay<10000 AND intervalFromLastPlay>1000 AND createdAt BETWEEN ? AND ? AND streamName=?) AS ReconnectCount,
  (SELECT COUNT(*) FROM statrecord WHERE createdAt >= ? AND createdAt < ?) AS RecordTotal,
  (SELECT COUNT(*) FROM statrecord WHERE isSucceed=1 AND createdAt >= ? AND createdAt < ?) AS RecordSuccessNum,
  (SELECT COUNT(*) FROM statplayback WHERE createdAt >= ? AND createdAt < ?) AS PlaybackTotal,
  (SELECT COUNT(*) FROM statplayback WHERE isSucceed=1 AND createdAt >= ? AND createdAt < ?) AS PlaybackSuccessNum
`,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime, streamPath,
				startTime, endTime,
				startTime, endTime,
				startTime, endTime,
				startTime, endTime).Scan(&tmp).Error; err != nil {
				tracer.LogWarn(tracer.ID_APP, "AggregateDailyStat error: %v", err)
				continue
			}
			if tmp.StreamTotal == 0 {
				tracer.LogWarn(tracer.ID_APP, "%s no pull request, skip to process!", field)
				continue
			}
			ss, exist := GetStreamState(streamPath)
			if !exist || ss != utils.StateActive {
				tracer.LogWarn(tracer.ID_APP, "%s:%s not active, skip to process! %v", field, streamPath, ss)
				continue
			}

			// 计算指标
			if tmp.StreamTotal > 0 {
				result.StreamSuccessRate = float64(tmp.StreamSuccessNum) / float64(tmp.StreamTotal) * 100
			}
			// 播放卡顿率按时长计算 (%)
			if tmp.PlayDurationTotal > 0 {
				result.LagRate = float64(tmp.LagDurationTotal) / float64(tmp.PlayDurationTotal) * 100
			}
			if tmp.RecordTotal > 0 {
				result.RecordSuccessRate = float64(tmp.RecordSuccessNum) / float64(tmp.RecordTotal) * 100
			}
			if tmp.StreamSuccessNum > 0 {
				// 按秒计算平均播放时长（playDuration 单位为毫秒），四舍五入到秒
				avgSeconds := math.Round(float64(tmp.PlayDurationTotal) / float64(tmp.StreamSuccessNum) / 1000.0)
				result.AvgPlayDuration = int(avgSeconds)
			}
			if tmp.PlaybackTotal > 0 {
				result.PlaybackSuccessRate = float64(tmp.PlaybackSuccessNum) / float64(tmp.PlaybackTotal) * 100
			}

			// 保存结果：StatDate 设为昨日的零点，便于按日期查询
			result.FieldName = field
			result.StreamPath = streamPath
			result.StatDate = startTime
			result.RequestStreamTotal = tmp.StreamTotal
			result.StreamSuccessNum = tmp.StreamSuccessNum
			result.RequestRecordTotal = tmp.RecordTotal
			result.RecordSuccessNum = tmp.RecordSuccessNum
			result.PlayDurationTotal = tmp.PlayDurationTotal
			result.LagCount = tmp.LagCount
			result.LagDurationTotal = tmp.LagDurationTotal
			result.PlaybackTotal = tmp.PlaybackTotal
			result.PlaybackSuccessNum = tmp.PlaybackSuccessNum

			err := db.Create(&result).Error
			if err != nil {
				tracer.LogWarn(tracer.ID_APP, "[DailyStat] insert failed: %v", err)
			} else {
				tracer.LogDebug(tracer.ID_APP,
					"[DailyStat] %s, section:%s -- %s", field, startTime.Format("2006-01-02 15:00:00"), endTime.Format("2006-01-02 15:00:00"))
			}

			// if result.RequestStreamTotal > 0 {
			// 	stat := MetricCardMsg{}
			// 	stat.FieldName = field
			// 	stat.StatHour = result.StatDate
			// 	stat.CreatedAt = result.CreatedAt
			// 	stat.AvgStreamSuccessRate = result.StreamSuccessRate
			// 	stat.AvgRecordSuccessRate = result.RecordSuccessRate
			// 	stat.RequestStreamTotal = result.RequestStreamTotal
			// 	stat.AvgLoadTime = int64(result.AvgLoadTime)
			// 	stat.ReconnectCount = result.ReconnectCount
			// 	textMsg := buildCardMessage(models.GetLarkSecret(), stat, 1)
			// 	err = postToLark(models.GetLarkWebhook(), textMsg)
			// 	if err != nil {
			// 		tracer.LogInfo(tracer.ID_APP, "DailyReport send to lark failure! %v", err)
			// 	}
			// }
			var chart *ChartSpec = nil
			var statDate string = result.StatDate.Format("**🕒 2006-01-02（Report Date）**")
			end := time.Now()
			start := end.Add(time.Duration(-15 * 24 * time.Hour)) // 最近15天趋势
			playReq, playSucc, avgSucc, err := BuildInteractiveChart(db, start, end, "day", streamPath, "")
			if err != nil {
				tracer.LogInfo(tracer.ID_APP, "build DailyReport failure! %v", err)
			} else {
				chart = BuildChart("PlaySuccessRate Trend", "{label}%", []ChartData{playSucc, avgSucc, playReq})
			}

			cardOps := LarkCardOptions{
				Title:     fmt.Sprintf("Daily Monitoring Report (%s)", field),
				HeaderTpl: "blue",
				Markdown:  statDate,
				Chart:     chart,
				DetailURL: utils.GetStatDetailUrl(),
			}

			if result.RequestStreamTotal > 0 {
				cols := []TableColumn{
					{Name: "metric", DisplayName: "Metric", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "value", DisplayName: "Current Value", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "range", DisplayName: "Normal Range", DataType: "lark_md", HorizontalAlign: "center"},
					{Name: "status", DisplayName: "Status", DataType: "lark_md", HorizontalAlign: "center"},
				}
				rows := []map[string]any{
					{"metric": "Play Success Rate", "value": fmt.Sprintf("%.2f%%", result.StreamSuccessRate), "range": "≥95%", "status": statusEmoji(result.StreamSuccessRate >= 95)},
					{"metric": "Recording Success Rate", "value": fmt.Sprintf("%.2f%%", result.RecordSuccessRate), "range": "≥95%", "status": statusEmoji(result.RecordSuccessRate >= 95)},
					{"metric": "Stream Requests", "value": fmt.Sprintf("%d", result.RequestStreamTotal), "range": "", "status": statusEmoji(result.RequestStreamTotal > 0)},
					{"metric": "Average Startup Time", "value": fmt.Sprintf("%.1f s", result.AvgLoadTime/1000), "range": "≤2 s", "status": statusEmoji(result.AvgLoadTime <= 2000)},
					{"metric": "Stream Break Count", "value": fmt.Sprintf("%d", result.ReconnectCount), "range": "≤10", "status": statusEmoji(result.ReconnectCount <= 10)},
				}
				cardOps.TableCols = cols
				cardOps.TableRows = rows
			}
			card := BuildLarkInteractiveCard(cardOps)

			if err = SendToLarkWebhook(models.GetLarkWebhook(), models.GetLarkSecret(), card); err != nil {
				tracer.LogInfo(tracer.ID_APP, "DailyReport send to lark failure! %v", err)
			} else {
				tracer.LogInfo(tracer.ID_APP, "DailyReport send to lark success!")
			}

		}
	}
}
