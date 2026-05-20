package tasks

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"backend/models"
	"backend/services"
	"backend/tracer"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func clearOverDatedDbRecords(days int) {

	tenDaysAgo := time.Now().AddDate(0, 0, -days)
	dbr := models.GetDbConn()

	result := dbr.Where("createdAt < ?", tenDaysAgo).Delete(&models.StartPlayStat{})
	tracer.LogWarn(tracer.ID_APP, "StartPlayStat:%d records deleted!", result.RowsAffected)
	result = dbr.Where("createdAt < ?", tenDaysAgo).Delete(&models.EndPlayStat{})
	tracer.LogWarn(tracer.ID_APP, "EndPlayStat:%d records deleted!", result.RowsAffected)
	result = dbr.Where("createdAt < ?", tenDaysAgo).Delete(&models.PlayLagStat{})
	tracer.LogWarn(tracer.ID_APP, "PlayLagStat:%d records deleted!", result.RowsAffected)
	result = dbr.Where("createdAt < ?", tenDaysAgo).Delete(&models.RecordStat{})
	tracer.LogWarn(tracer.ID_APP, "RecordStat:%d records deleted!", result.RowsAffected)

}

func MonitorMainThread() {
	defer tracer.TryException()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	preserveDays := 90
	lastClearTime := time.Now()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	tracer.LogInfo(tracer.ID_APP, "MonitorMainThread started, preserveDays=%d", preserveDays)

	var checkDbCount int = 0
	services.CheckStreamStatus()
	for {
		select {
		case <-ticker.C:
			services.CheckStreamStatus()

			numGoroutine := runtime.NumGoroutine()
			percs, _ := cpu.Percent(0, false)
			memStat, _ := mem.VirtualMemory()
			cpuV := 0.0
			if len(percs) > 0 {
				cpuV = percs[0]
			}
			cpuText := fmt.Sprintf("CPU: %.1f%%", cpuV)
			memText := fmt.Sprintf("Memory: %.1f%% (%d MB)",
				memStat.UsedPercent,
				memStat.Used/1024/1024,
			)
			tracer.LogDebug(tracer.ID_APP, "numGoroutine=%d, %s, %s", numGoroutine, cpuText, memText)

			//5m检查一次db connection是否正常
			checkDbCount++
			if checkDbCount >= 5 {
				models.ActiveDbConn()
				checkDbCount = 0
			}

			// 2️⃣ 每天清理旧数据（间隔24小时）
			now := time.Now()
			if now.Sub(lastClearTime) >= 24*time.Hour {
				tracer.LogWarn(tracer.ID_APP, "clear db records older than %d days...", preserveDays)
				clearOverDatedDbRecords(preserveDays)
				lastClearTime = now
			}

		case <-interrupt:
			tracer.LogWarn(tracer.ID_APP, "MonitorMainThread received interrupt, exiting.")
			return
		}
	}
}
