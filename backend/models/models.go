package models

import "time"

/*
表格模型定义只能用于查询序列化，不要用于create table.
*/
type StartPlayStat struct {
	ID                   uint64    `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	GameTypeId           string    `gorm:"type:varchar(64);column:gameTypeId" json:"gameTypeId"`
	GameUserId           string    `gorm:"type:varchar(64);column:gameUserId;not null;index:idx_user" json:"gameUserId"`
	GameRound            string    `gorm:"type:varchar(64);column:gameRound" json:"gameRound"`
	GeoLocation          string    `gorm:"type:varchar(64);column:geoLocation" json:"geoLocation"`
	CdnName              string    `gorm:"type:varchar(64);default:'tencent';column:cdnName;index:idx_cdn_stream,priority:1" json:"cdnName"`
	StreamName           string    `gorm:"type:varchar(128);column:streamName;index:idx_cdn_stream,priority:2" json:"streamName"`
	Protocol             string    `gorm:"type:varchar(32);default:'webrtc';column:protocol" json:"protocol"`
	Quality              string    `gorm:"type:varchar(32);default:'1';column:quality" json:"quality"`
	IsSucceed            bool      `gorm:"column:isSucceed;default:false;not null" json:"isSucceed"`
	CanHevc              bool      `gorm:"-" json:"canHevc"`
	Reason               string    `gorm:"type:varchar(255);column:reason" json:"reason"`
	UserAgent            string    `gorm:"type:varchar(255);column:userAgent" json:"userAgent"`
	IntervalFromLastPlay int64     `gorm:"column:intervalFromLastPlay" json:"intervalFromLastPlay"`
	WaitMiliTime         int64     `gorm:"column:waitMiliTime" json:"waitMiliTime"`
	RetryCount           int       `gorm:"column:retryCount" json:"retryCount"`
	CreatedAt            time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
}

type EndPlayStat struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	GameTypeId   string    `gorm:"type:varchar(64);column:gameTypeId" json:"gameTypeId"`
	GameUserId   string    `gorm:"type:varchar(64);column:gameUserId;not null;index:idx_user" json:"gameUserId"`
	GameRound    string    `gorm:"type:varchar(64);column:gameRound" json:"gameRound"`
	GeoLocation  string    `gorm:"type:varchar(64);column:geoLocation" json:"geoLocation"`
	CdnName      string    `gorm:"type:varchar(64);default:'tencent';column:cdnName;index:idx_cdn_stream,priority:1" json:"cdnName"`
	StreamName   string    `gorm:"type:varchar(128);column:streamName;index:idx_cdn_stream,priority:2" json:"streamName"`
	PlayDuration int64     `gorm:"column:playDuration" json:"playDuration"` // 毫秒
	CreatedAt    time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
}

type PlayLagStat struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	GameTypeId  string    `gorm:"type:varchar(64);column:gameTypeId" json:"gameTypeId"`
	GameUserId  string    `gorm:"type:varchar(64);column:gameUserId;not null;index:idx_user" json:"gameUserId"`
	GameRound   string    `gorm:"type:varchar(64);column:gameRound" json:"gameRound"`
	GeoLocation string    `gorm:"type:varchar(64);column:geoLocation" json:"geoLocation"`
	CdnName     string    `gorm:"type:varchar(64);default:'tencent';column:cdnName;index:idx_cdn_stream,priority:1" json:"cdnName"`
	StreamName  string    `gorm:"type:varchar(128);column:streamName;index:idx_cdn_stream,priority:2" json:"streamName"`
	LagDuration int64     `gorm:"column:lagDuration" json:"lagDuration"` // 毫秒
	CreatedAt   time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
}

type RecordStat struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement;type:bigint unsigned;column:id" json:"id"`
	GameTypeId     string    `gorm:"type:varchar(64);column:gameTypeId;index:idx_gameType" json:"gameTypeId"`
	GameRound      string    `gorm:"type:varchar(64);column:gameRound;index:idx_round" json:"gameRound"`
	IsSucceed      bool      `gorm:"column:isSucceed" json:"isSucceed"`
	Reason         string    `gorm:"type:varchar(256);column:reason" json:"reason"`
	PlaybackUri    string    `gorm:"type:varchar(512);column:playbackUri" json:"playbackUri"`
	RecordDuration int64     `gorm:"column:recordDuration" json:"recordDuration"` // 秒
	CreatedAt      time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
}

// PlaybackStat 回放成功率上报表
type PlaybackStat struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;type:bigint unsigned;column:id" json:"id"`
	GameTypeId  string    `gorm:"type:varchar(64);column:gameTypeId" json:"gameTypeId"`
	GameUserId  string    `gorm:"type:varchar(64);column:gameUserId;not null;index:idx_user" json:"gameUserId"`
	GeoLocation string    `gorm:"type:varchar(128);column:geoLocation" json:"geoLocation"`
	PlaybackUri string    `gorm:"type:varchar(512);column:playbackUri" json:"playbackUri"` // 回放视频地址
	IsSucceed   bool      `gorm:"column:isSucceed" json:"isSucceed"`                       // 是否回放成功
	Reason      string    `gorm:"type:varchar(256);column:reason" json:"reason"`           // 失败原因
	CreatedAt   time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
}

// FieldRoundStat 表示现场+局次的基本统计信息
type FieldRoundStat struct {
	ID     uint64 `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	Status string `gorm:"column:status;type:enum('going','done','cancel');default:'done';comment:'局状态: going/done/cancel'" json:"status"`

	// 现场名称（如：sabadoRoom / 3DRushHall）。一般这个字段都不填，游戏局号一定要可以区分现场。
	FieldName string `gorm:"type:varchar(128);default:'GSP2W_FIELD';column:fieldName;index:idx_field_round,priority:1" json:"fieldName"`

	// 游戏局号（如：Round001、20251024-001）
	RoundId string `gorm:"type:varchar(128);uniqueIndex;index:idx_field_round,priority:2;index:idx_round;column:roundId;not null" json:"roundId"`

	// 本局开始时间
	RoundStartTime time.Time `gorm:"column:roundStartTime" json:"roundStartTime"`

	// 本局结束时间
	RoundEndTime *time.Time `gorm:"column:roundEndTime" json:"roundEndTime"`

	// 创建时间、更新时间
	CreatedAt time.Time `gorm:"autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

type HourlyVideoStat struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	FieldName  string `gorm:"type:varchar(128);column:fieldName" json:"fieldName"`
	StreamPath string `gorm:"type:varchar(128);column:streamPath" json:"streamPath"`

	StatHour             time.Time `gorm:"type:datetime;column:statHour;index:idx_hour" json:"statHour"`
	RequestStreamTotal   int64     `gorm:"type:bigint;column:requestStreamTotal" json:"requestStreamTotal"`
	StreamSuccessNum     int64     `gorm:"type:bigint;column:streamSuccessNum" json:"streamSuccessNum"`
	AvgLoadTime          float64   `gorm:"type:float;column:avgLoadTime;comment:平均加载时间" json:"avgLoadTime"`
	ReconnectCount       int64     `gorm:"type:bigint;column:reconnectCount" json:"reconnectCount"`
	PlayDurationTotal    int64     `gorm:"type:bigint;column:playDurationTotal" json:"playDurationTotal"`
	AvgPlayDuration      float64   `gorm:"type:float;column:avgPlayDuration" json:"avgPlayDuration"`
	LagDurationTotal     int64     `gorm:"type:bigint;column:lagDurationTotal" json:"lagDurationTotal"`
	LagCount             int64     `gorm:"type:bigint;column:lagCount" json:"lagCount"`
	AvgLagRate           float64   `gorm:"type:float;column:avgLagRate;comment:平均卡顿率(%)" json:"avgLagRate"`
	AvgStreamSuccessRate float64   `gorm:"type:float;column:avgStreamSuccessRate;comment:平均拉流成功率(%)" json:"avgStreamSuccessRate"`

	RecordTotal            int64     `gorm:"type:bigint;column:recordTotal" json:"recordTotal"`
	RecordSuccessNum       int64     `gorm:"type:bigint;column:recordSuccessNum" json:"recordSuccessNum"`
	PlaybackTotal          int64     `gorm:"type:bigint;column:playbackTotal" json:"playbackTotal"`
	PlaybackSuccessNum     int64     `gorm:"type:bigint;column:playbackSuccessNum" json:"playbackSuccessNum"`
	AvgRecordSuccessRate   float64   `gorm:"type:float;column:avgRecordSuccessRate;comment:平均录像成功率(%)" json:"avgRecordSuccessRate"`
	AvgPlaybackSuccessRate float64   `gorm:"type:float;column:avgPlaybackSuccessRate;comment:平均回放成功率(%)" json:"avgPlaybackSuccessRate"`
	CreatedAt              time.Time `gorm:"type:datetime;autoCreateTime;column:createdAt" json:"createdAt"`
}

type DailyVideoStat struct {
	ID       uint64    `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	StatDate time.Time `gorm:"type:date;uniqueIndex;index:idx_statDate;column:statDate;not null" json:"statDate"`

	FieldName  string `gorm:"type:varchar(128);column:fieldName" json:"fieldName"`
	StreamPath string `gorm:"type:varchar(128);column:streamPath" json:"streamPath"`

	// 拉流成功率
	RequestStreamTotal int64   `gorm:"type:int;column:requestStreamTotal" json:"requestStreamTotal"`
	StreamSuccessNum   int64   `gorm:"type:int;column:streamSuccessNum" json:"streamSuccessNum"`
	StreamSuccessRate  float64 `gorm:"type:decimal(6,2);column:streamSuccessRate" json:"streamSuccessRate"`
	AvgLoadTime        float64 `gorm:"type:float;column:avgLoadTime;comment:平均加载时间" json:"avgLoadTime"`
	ReconnectCount     int64   `gorm:"type:bigint;column:reconnectCount" json:"reconnectCount"`

	// 播放卡顿率部分（按时长计算）
	PlayDurationTotal int64   `gorm:"type:bigint;column:playDurationTotal" json:"playDurationTotal"` // 播放总时长（毫秒）
	LagDurationTotal  int64   `gorm:"type:bigint;column:lagDurationTotal" json:"lagDurationTotal"`   // 卡顿总时长（毫秒）
	LagCount          int64   `gorm:"type:int;column:lagCount" json:"lagCount"`                      // 卡顿总次数
	LagRate           float64 `gorm:"type:decimal(6,2);column:lagRate" json:"lagRate"`               // 播放卡顿率 %
	AvgPlayDuration   int     `gorm:"type:int;column:avgPlayDuration" json:"avgPlayDuration"`        // 平均播放时长（秒）

	// 录像成功率
	RequestRecordTotal int64   `gorm:"type:int;column:requestRecordTotal" json:"requestRecordTotal"`
	RecordSuccessNum   int64   `gorm:"type:int;column:recordSuccessNum" json:"recordSuccessNum"`
	RecordSuccessRate  float64 `gorm:"type:decimal(6,2);column:recordSuccessRate" json:"recordSuccessRate"`

	// 回放成功率
	PlaybackTotal       int64   `gorm:"type:int;column:playbackTotal" json:"playbackTotal"`                      // 回放总次数
	PlaybackSuccessNum  int64   `gorm:"type:int;column:playbackSuccessNum" json:"playbackSuccessNum"`            // 回放成功次数
	PlaybackSuccessRate float64 `gorm:"type:decimal(6,2);column:playbackSuccessRate" json:"playbackSuccessRate"` // 回放成功率 %

	CreatedAt time.Time `gorm:"type:datetime;autoCreateTime;column:createdAt" json:"createdAt"`
	UpdatedAt time.Time `gorm:"type:datetime;autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

// RoundVideoStat 每局(按 RoundId)聚合视频统计
type RoundVideoStat struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`

	// 局号标识（唯一）
	RoundId string `gorm:"type:varchar(128);uniqueIndex;index:idx_round;column:roundId;not null" json:"roundId"`

	// 对应现场
	FieldName string `gorm:"type:varchar(128);column:fieldName" json:"fieldName"`

	// 开始/结束时间
	RoundStartTime time.Time `gorm:"column:roundStartTime" json:"roundStartTime"`
	RoundEndTime   time.Time `gorm:"column:roundEndTime" json:"roundEndTime"`

	// 直播时长(秒)
	LiveDuration int64 `gorm:"column:liveDuration" json:"liveDuration"`

	// 拉流统计
	RequestStreamTotal int64   `gorm:"column:requestStreamTotal" json:"requestStreamTotal"`
	StreamSuccessNum   int64   `gorm:"column:streamSuccessNum" json:"streamSuccessNum"`
	ReconnectCount     int64   `gorm:"column:reconnectCount" json:"reconnectCount"`
	AvgLoadTime        float64 `gorm:"type:double;column:avgLoadTime" json:"avgLoadTime"` // 平均开播时延(ms)

	// 录像相关
	RecordDurationTotal int64   `gorm:"column:recordDurationTotal" json:"recordDurationTotal"`             // 所有录像时长(秒)
	VideoCompletionRate float64 `gorm:"type:double;column:videoCompletionRate" json:"videoCompletionRate"` // 录像完整率(%)

	// 拉流成功率 (%)
	StreamSuccessRate float64 `gorm:"type:double;column:streamSuccessRate" json:"streamSuccessRate"`

	CreatedAt time.Time `gorm:"type:datetime;autoCreateTime;column:createdAt;index:idx_createdAt" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

// FieldStreamsInfo 存储每个现场及其对应的流路径
type FieldStreamsInfo struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`

	// 现场名称，如 sabadoRoom、rushHall 等
	FieldName string `gorm:"type:varchar(128);uniqueIndex;index:idx_field;column:fieldName;not null" json:"fieldName"`

	// 对应的流路径（可以是 RTMP、WebRTC、HLS 等）
	StreamPath string `gorm:"type:varchar(256);column:streamPath;not null" json:"streamPath"`

	// 备注信息（可选）
	Description string `gorm:"type:varchar(256);column:description" json:"description"`

	CreatedAt time.Time `gorm:"autoCreateTime;column:createdAt" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

// FieldsInfo 存储每个现场及其对应的流路径
type FieldsInfo struct {
	ID uint64 `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`

	// 现场名称，如 sabadoRoom、rushHall 等
	FieldName string `gorm:"type:varchar(128);uniqueIndex;index:idx_field;column:fieldName;not null" json:"fieldName"`
	FieldType string `gorm:"type:varchar(128);column:fieldType;not null" json:"fieldType"`

	// 备注信息（可选）
	Description string `gorm:"type:varchar(256);column:description" json:"description"`

	CreatedAt time.Time `gorm:"autoCreateTime;column:createdAt" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

/*
| 场景             | 状态变化                   | 操作逻辑                                                                              |
| -------------- | ---------------------- | --------------------------------------------------------------------------------- |
| 流第一次推送         | `status = "active"`    | 插入新记录，设置 `startTime = now()`                                                      |
| 检测断流（如无心跳超过阈值） | `status = "broken"`    | 更新 `breakStartTime = now()`，`totalBreakCount++`                                   |
| 检测恢复推流         | `status = "recovered"` | 更新 `breakRecoverTime = now()`，`breakDuration = breakRecoverTime - breakStartTime` |
| 流彻底结束          | `status = "ended"`     | 更新 `duration = endTime - startTime`                                               |
| 定期心跳上报         | 不改变状态                  | 仅更新 `lastHeartbeat = now()`                                                       |
*/
// StatStreamStatus 表：记录每一路推流的生命周期状态
type StatStreamStatus struct {
	ID               uint64     `gorm:"column:id;primaryKey;autoIncrement;type:bigint;comment:'自增主键'" json:"id"`
	StreamPath       string     `gorm:"column:streamPath;type:varchar(128);uniqueIndex:uniq_stream;not null;comment:'流名称或路径'" json:"streamPath"`
	StartTime        *time.Time `gorm:"column:startTime;type:datetime;comment:'推流启动时间'" json:"startTime"`
	BreakStartTime   *time.Time `gorm:"column:breakStartTime;type:datetime;comment:'中断开始时间'" json:"breakStartTime,omitempty"`
	BreakRecoverTime *time.Time `gorm:"column:breakRecoverTime;type:datetime;comment:'中断恢复时间'" json:"breakRecoverTime,omitempty"`
	Status           string     `gorm:"column:status;type:enum('active','broken','recovered','ended');default:'active';index:idx_status;comment:'流状态: active/broken/recovered/ended'" json:"status"`
	Duration         int64      `gorm:"column:duration;type:bigint;default:0;comment:'推流持续时间(ms)'" json:"duration"`
	BreakDuration    int64      `gorm:"column:breakDuration;type:bigint;default:0;comment:'中断持续时间(ms)'" json:"breakDuration"`
	TotalBreakCount  int        `gorm:"column:totalBreakCount;type:int;default:0;comment:'累计中断次数'" json:"totalBreakCount"`
	LastHeartbeat    time.Time  `gorm:"column:lastHeartbeat;type:datetime;default:CURRENT_TIMESTAMP;index:idx_lastHeartbeat;not null;comment:'最近收到心跳时间'" json:"lastHeartbeat"`
	CreatedAt        time.Time  `gorm:"column:createdAt;type:datetime;autoCreateTime;comment:'创建时间'" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"column:updatedAt;type:datetime;autoUpdateTime;comment:'更新时间'" json:"updatedAt"`
}

// 明确指定 GORM 表名，避免默认的 snake_case + plural 导致找不到表
func (StartPlayStat) TableName() string    { return "statstartplay" }
func (EndPlayStat) TableName() string      { return "statendplay" }
func (PlayLagStat) TableName() string      { return "statplaylag" }
func (RecordStat) TableName() string       { return "statrecord" }
func (PlaybackStat) TableName() string     { return "statplayback" }
func (FieldRoundStat) TableName() string   { return "statfieldround" }
func (StatStreamStatus) TableName() string { return "statStreamStatus" }
func (HourlyVideoStat) TableName() string  { return "videostat_hourly" }
func (DailyVideoStat) TableName() string   { return "videostat_daily" }
func (RoundVideoStat) TableName() string   { return "videostat_round" }
func (FieldStreamsInfo) TableName() string { return "info_field_streams" }
func (FieldsInfo) TableName() string       { return "info_fields" }
