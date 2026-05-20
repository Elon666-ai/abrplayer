package services

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"backend/models"
	"backend/tracer"
	"backend/utils"
)

var mapInputParameter map[string]string = make(map[string]string)
var lockInput = &sync.RWMutex{}

func GetInputsStatus() string {
	lockInput.RLock()
	defer lockInput.RUnlock()

	var inputs string = "{"
	var n int = 0
	for uri, parameter := range mapInputParameter {
		inputs += fmt.Sprintf("\"%s\":\"%s\"", uri, parameter)
		n += 1
		if n < len(mapInputParameter) {
			inputs += ","
		}
	}
	inputs += "}"
	return inputs
}

func SetInputParameter(uri, parameter string) {
	lockInput.Lock()
	defer lockInput.Unlock()

	mapInputParameter[uri] = parameter
}

// "active" --> "broken" --> "recovered" --> "ended"
func OnStreamStart(streamPath string) {
	db := models.GetDbConn()
	now := time.Now()

	// 若已存在该流，重置状态
	var record models.StatStreamStatus
	if err := db.Where("streamPath = ?", streamPath).First(&record).Error; err == nil {
		record.Status = "active"
		record.StartTime = &now
		record.LastHeartbeat = now
		record.TotalBreakCount = 0
		record.BreakDuration = 0
		db.Save(&record)
	} else {
		record = models.StatStreamStatus{
			StreamPath:    streamPath,
			Status:        "active",
			StartTime:     &now,
			LastHeartbeat: now,
		}
		db.Create(&record)
	}
}

func OnStreamBroken(streamPath string) int64 {
	db := models.GetDbConn()
	now := time.Now()
	var brokenDur int64 = 0

	var s models.StatStreamStatus
	if err := db.Where("streamPath = ?", streamPath).First(&s).Error; err == nil {
		if s.Status == "active" {
			s.BreakStartTime = &now
		}
		s.Status = "broken"
		if s.BreakStartTime != nil {
			d := time.Since(*s.BreakStartTime).Seconds()
			s.BreakDuration = int64(d)
		}
		db.Save(&s)
		brokenDur = s.BreakDuration
		tracer.LogWarn(tracer.ID_APP, "%s broken: %d", streamPath, brokenDur)
		return brokenDur
	} else {
		tracer.LogWarn(tracer.ID_APP, "%s not actived yet!", streamPath)
		return -1
	}
}

func OnStreamRecover(streamPath string) {
	db := models.GetDbConn()
	now := time.Now()

	var s models.StatStreamStatus
	if err := db.Where("streamPath = ?", streamPath).First(&s).Error; err == nil {
		if s.Status == "broken" {
			s.Status = "recovered"
			s.BreakRecoverTime = &now
			s.BreakDuration = 0
			s.LastHeartbeat = now
			s.TotalBreakCount += 1
			db.Save(&s)
		} else {
			// 已经恢复或活跃，刷新心跳
			s.LastHeartbeat = now
			db.Save(&s)
		}
	}
}

func OnStreamActive(streamPath string) {
	db := models.GetDbConn()
	if db == nil {
		return
	}

	now := time.Now()

	var s models.StatStreamStatus
	err := db.Where("streamPath = ?", streamPath).First(&s).Error

	if err != nil {
		// 没找到记录，创建新记录
		s = models.StatStreamStatus{
			StreamPath:    streamPath,
			Status:        "active",
			LastHeartbeat: now,
		}
		db.Create(&s)
		return
	}

	// 已存在记录，更新状态
	if s.Status == "recovered" || s.Status == "broken" || s.Status == "ended" {
		s.Status = "active"
	}

	// 安全计算中断时长
	if s.BreakStartTime != nil && !s.BreakStartTime.IsZero() {
		duration := time.Since(*s.BreakStartTime).Seconds() // 秒数更直观
		s.Duration = int64(duration)
	}

	s.LastHeartbeat = now

	if err := db.Save(&s).Error; err != nil {
		fmt.Printf("[WARN] failed to update stream active info: %v\n", err)
	}
}

func OnStreamEnd(streamPath string) {
	db := models.GetDbConn()
	now := time.Now()

	var s models.StatStreamStatus
	if err := db.Where("streamPath = ?", streamPath).First(&s).Error; err == nil {
		s.Status = "ended"
		s.Duration = now.Sub(*s.StartTime).Milliseconds()
		db.Save(&s)
	}
}

// StreamStatus 记录单路流的状态与时间信息
type StreamStatus struct {
	State          string        `json:"state"`
	LastChangeTime time.Time     `json:"lastChangeTime"`
	BreakStartTime time.Time     `json:"breakStartTime"`
	BreakDuration  time.Duration `json:"breakDuration"`
	RecoverTime    time.Time     `json:"recoverTime"`

	FailCount int `json:"-"` // 新增：连续失败计数
}

// 并发安全状态表
var (
	streamStates = make(map[string]*StreamStatus)
	lock         = &sync.RWMutex{}
)

// SetStreamState 设置或更新流状态（线程安全）
func SetStreamState(streamID, state string) {
	lock.Lock()
	defer lock.Unlock()

	st, ok := streamStates[streamID]
	if !ok {
		st = &StreamStatus{}
		streamStates[streamID] = st
	}

	now := time.Now()

	switch state {
	case utils.StateActive:
		st.State = utils.StateActive
		st.LastChangeTime = now
		st.BreakStartTime = time.Time{}
		st.BreakDuration = 0

	case utils.StateBroken:
		st.State = utils.StateBroken
		st.LastChangeTime = now
		st.BreakStartTime = now

	case utils.StateRecovered:
		st.State = utils.StateRecovered
		st.RecoverTime = now
		if !st.BreakStartTime.IsZero() {
			st.BreakDuration = now.Sub(st.BreakStartTime)
		}
		st.LastChangeTime = now

	case utils.StateEnded:
		st.State = utils.StateEnded
		st.LastChangeTime = now
	}
}

// --- 新增（连续失败计数逻辑） ---

// MarkStreamFail 连续失败计数，返回 true 表示连续两次失败，应判定为 broken
func MarkStreamFail(streamID string) bool {
	lock.Lock()
	defer lock.Unlock()

	st, ok := streamStates[streamID]
	if !ok {
		st = &StreamStatus{}
		streamStates[streamID] = st
	}

	st.FailCount++

	// 只有连续 2 次失败才认为 broken
	if st.FailCount >= 2 {
		st.State = utils.StateBroken
		st.LastChangeTime = time.Now()
		if st.BreakStartTime.IsZero() {
			st.BreakStartTime = time.Now()
		}
		return true
	}
	return false
}

// ResetFailCounter 成功时清零失败计数
func ResetFailCounter(streamID string) {
	lock.Lock()
	defer lock.Unlock()

	if st, ok := streamStates[streamID]; ok {
		st.FailCount = 0
	}
}

// GetStreamState 获取流当前状态（线程安全）
func GetStreamState(streamID string) (state string, exists bool) {
	lock.RLock()
	defer lock.RUnlock()

	st, ok := streamStates[streamID]
	if !ok {
		return "", false
	}
	return st.State, true
}

// FieldStreamMap 定义 FieldName → 多个 StreamPath 的映射
var FieldStreamMap = make(map[string][]string)

// LoadFieldStreamMapping 从数据库加载 FieldsInfo 和 FieldStreamsInfo 构建映射
func LoadFieldStreamMapping() error {
	var fields []models.FieldsInfo
	var streams []models.FieldStreamsInfo
	db := models.GetDbConn()

	// 查询所有现场
	if err := db.Find(&fields).Error; err != nil {
		return err
	}

	// 查询所有流路径
	if err := db.Find(&streams).Error; err != nil {
		return err
	}

	// 初始化映射表
	tmpMap := make(map[string][]string)
	for _, f := range fields {
		tmpMap[f.FieldName] = []string{}
	}

	// 将 streamPath 关联到对应现场
	for _, s := range streams {
		tmpMap[s.FieldName] = append(tmpMap[s.FieldName], s.StreamPath)
	}

	FieldStreamMap = tmpMap

	tracer.LogInfo(tracer.ID_APP, "[INIT] FieldStreamMap loaded successfully, total %d fields", len(FieldStreamMap))
	for name, list := range FieldStreamMap {
		tracer.LogInfo(tracer.ID_APP, "  > %s : %v", name, list)
	}
	return nil
}

func CheckStreamStatus() {
	for field, streams := range FieldStreamMap {
		for _, uri := range streams {
			txUri, err := ConvertRTMPURL(uri)
			if err != nil {
				tracer.LogWarn(tracer.ID_APP, "%s: %v", uri, err)
				SetInputParameter(uri, "-1")
				continue
			}
			streamPath := utils.ExtractStreamPath(uri)

			info, err := FfprobeStreamInfo2(txUri)
			if err != nil {
				tracer.LogWarn(tracer.ID_APP, "%s: %v", uri, err)
				SetInputParameter(uri, "-2")

				// === 新逻辑：连续 2 次失败才 broken ===
				if MarkStreamFail(streamPath) {
					brokenDur := OnStreamBroken(streamPath)
					if brokenDur >= 0 && (brokenDur <= 600 || brokenDur%3600 == 0) {
						go TriggerAlarmToLark(utils.AlarmStreamBroken, AlertInfo{
							FieldName:             field,
							StreamPath:            streamPath,
							DisconnectionDuration: time.Duration(brokenDur) * time.Second,
						})
					}
					// 如果这个流没有激活过，就不触发告警！
				}

				continue
			}

			// ffprobe 成功
			ResetFailCounter(streamPath)

			tracer.LogInfo(tracer.ID_APP, "%s:%s", uri, info)
			SetInputParameter(uri, info)

			if state, ok := GetStreamState(streamPath); ok {
				if state == utils.StateBroken {
					SetStreamState(streamPath, utils.StateActive)
					OnStreamRecover(streamPath)
					PostTextToLark(fmt.Sprintf("[%s]%s is back to normal! Streaming Status: %s", field, streamPath, statusEmoji(true)))
				} else {
					OnStreamActive(streamPath)
				}
			} else {
				SetStreamState(streamPath, utils.StateActive)
				OnStreamStart(streamPath)
				//PostTextToLark(fmt.Sprintf("[%s]%s checked pass! Streaming Status: %s", field, streamPath, statusEmoji(true)))
				tracer.LogInfo(tracer.ID_APP, "remind: [%s]%s checked pass! Streaming Status: %s", field, streamPath, statusEmoji(true))
			}

			if !strings.Contains(info, "Video:codec=h264") || !strings.Contains(info, "Audio:codec=aac") {
				tracer.LogWarn(tracer.ID_APP, "warn: %s video-settings wrong! %s:%s", field, uri, info)
			}

		}
	}
}
