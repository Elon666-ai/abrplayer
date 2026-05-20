package tasks

import (
	"time"
	"backend/models"
	"backend/services"
	"backend/tracer"
)

/* Hourly Monitoring Data Push（每小时监控数据推送）
| 指标                       | 含义           | 计算逻辑                                        |
| ------------------------ | ------------ | ------------------------------------------- |
| `requestStreamTotal`     | 拉流请求总数       | COUNT(startplaystat)                        |
| `avgLoadTime`            | 平均拉流加载时延(ms) | AVG(waitMiliTime)                           |
| `reconnectCount`         | 重连次数         | 连续两次同 `gameUserId` + `streamName` 且间隔 <10s  |
| `avgStreamSuccessRate`   | 平均拉流成功率      | streamSuccessNum / requestStreamTotal       |
| `avgLagRate`             | 平均卡顿率        | lagDurationTotal / playDurationTotal        |
| `lagCount`               | 严重卡顿次数       | COUNT(lagDuration > 2000)                   |
| `avgPlayDuration`        | 平均播放时长(s)    | playDurationTotal / streamSuccessNum / 1000 |
| `avgRecordSuccessRate`   | 平均录像成功率      | recordSuccessNum / recordTotal              |
| `avgPlaybackSuccessRate` | 平均回放成功率      | playbackSuccessNum / playbackTotal          |
| `recordTotal`            | 录像次数         | COUNT(recordstat)                           |
| `playbackTotal`          | 回放次数         | COUNT(playbackstat)                         |

*/
// RunDailyVideoStat 每天 00:00:01 聚合统计昨天的
func ScheduleDailyStat() {
	defer tracer.TryException()

	go func() {
		for {
			now := time.Now()

			// 计算下一次执行时间（每小时的59分59秒）
			next := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 59, 59, 0, now.Location())

			// 如果当前已经过了59:59，就等到下一小时
			if next.Before(now) {
				next = next.Add(time.Hour)
			}

			duration := next.Sub(now)
			tracer.LogDebug(tracer.ID_APP, "[CRON] Hourly stat scheduled at %v (in %v)", next.Format("15:04:05"), duration)

			time.Sleep(duration) // 等待到执行时刻

			// 每个 tick 独立捕获 panic，防止杀死调度循环
			func() {
				defer tracer.TryException()
				db := models.GetDbConn()
				services.AggregateHourlyStat(db)
				tracer.LogInfo(tracer.ID_APP, "[CRON] Hourly stat job completed at %v", time.Now().Format("2006-01-02 15:04:05"))
			}()

			// 稍作等待，确保跨整点后再进入下一轮
			time.Sleep(2 * time.Second)
		}
	}()

	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 00, 00, 01, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			sleepDur := time.Until(next)
			tracer.LogDebug(tracer.ID_APP, "[Scheduler] next daily stat at %v", next)
			time.Sleep(sleepDur)

			func() {
				defer tracer.TryException()
				db := models.GetDbConn()
				services.AggregateDailyStat(db)
				tracer.LogInfo(tracer.ID_APP, "[Scheduler] daily stat completed at %v", time.Now().Format("2006-01-02 15:04:05"))
			}()
		}
	}()
}
