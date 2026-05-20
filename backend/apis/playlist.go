package apis

import (
	"net/http"
	"time"
	"backend/models"

	"github.com/gin-gonic/gin"
)

type PlayListReq struct {
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
	IsSucceed   *bool  `json:"isSucceed"`
	MinLoadTime int64  `json:"minLoadTime"`
	MaxLoadTime int64  `json:"maxLoadTime"`
	AgentType   string `json:"agentType"`
	CdnName     string `json:"cdnName"`
	StreamName  string `json:"streamName"` // ⭐新增 StreamName 过滤器
	Page        int    `json:"page"`
	PageSize    int    `json:"pageSize"`
}

/*
* /api/stats/play/list
* StartPlayStat 全字段返回
* 支持：
*   streamName
*   拉流成功 isSucceed
*   waitMiliTime 区间
*   AgentType（模糊）
*   cdnName
*   时间区间
* pageSize 默认 200
 */
func ApiStatsPlayList(c *gin.Context) {
	var req PlayListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.HttpRespMsg{Code: 400, Msg: err.Error()})
		return
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 200
	}

	db := models.GetDbConn()
	query := db.Model(&models.StartPlayStat{})

	// 时间区间（校验格式，防止注入怪异字符串）
	if req.StartTime != "" && req.EndTime != "" {
		st, errSt := time.Parse("2006-01-02 15:04:05", req.StartTime)
		et, errEt := time.Parse("2006-01-02 15:04:05", req.EndTime)
		if errSt != nil || errEt != nil {
			c.JSON(http.StatusBadRequest, models.HttpRespMsg{Code: 400, Msg: "invalid startTime/endTime format, expect 2006-01-02 15:04:05"})
			return
		}
		query = query.Where("createdAt BETWEEN ? AND ?", st, et)
	}

	// StreamName
	if req.StreamName != "" {
		query = query.Where("streamName = ?", req.StreamName)
	}

	// 拉流成功
	if req.IsSucceed != nil {
		query = query.Where("isSucceed = ?", *req.IsSucceed)
	}

	// 加载时延区间
	if req.MinLoadTime > 0 || req.MaxLoadTime > 0 {
		query = query.Where("waitMiliTime BETWEEN ? AND ?", req.MinLoadTime, req.MaxLoadTime)
	}

	// cdnName
	if req.CdnName != "" {
		query = query.Where("cdnName LIKE ?", "%"+req.CdnName+"%")
	}

	// AgentType -> userAgent
	if req.AgentType != "" {
		query = query.Where("userAgent LIKE ?", "%"+req.AgentType+"%")
	}

	var total int64
	query.Count(&total)

	var items []models.StartPlayStat
	query.Order("createdAt DESC").
		Limit(req.PageSize).
		Offset((req.Page - 1) * req.PageSize).
		Find(&items)

	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: 200,
		Msg:  "OK",
		Data: models.ListData{
			Items:    items,
			Total:    total,
			Page:     req.Page,
			PageSize: req.PageSize,
		},
	})
}

type TrendReq struct {
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
	CdnName    string `json:"cdnName"`
	StreamName string `json:"streamName"` // ⭐新增 StreamName
}

/*
* /api/stats/play/trend
* 来源表：HourlyVideoStat
* 支持：
*   streamPath (streamName)
*   cdnName (忽略，因为 HourlyVideoStat 无 cdnName 字段)
*   时间区间 1~30 天
 */
func ApiStatsPlayTrend(c *gin.Context) {
	var req TrendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.HttpRespMsg{Code: 400, Msg: err.Error()})
		return
	}

	start, err1 := time.Parse("2006-01-02 15:04:05", req.StartTime)
	end, err2 := time.Parse("2006-01-02 15:04:05", req.EndTime)
	if err1 != nil || err2 != nil {
		c.JSON(400, models.HttpRespMsg{Code: 400, Msg: "invalid date format"})
		return
	}

	diff := end.Sub(start).Hours() / 24
	if diff < 1 || diff > 30 {
		c.JSON(400, models.HttpRespMsg{Code: 400, Msg: "time range must be between 1 and 30 days"})
		return
	}

	db := models.GetDbConn()
	query := db.Model(&models.HourlyVideoStat{})

	// 时间区间过滤
	if req.StartTime != "" && req.EndTime != "" {
		query = query.Where("createdAt BETWEEN ? AND ?", req.StartTime, req.EndTime)
	}

	// ⭐ 按 streamPath 过滤
	if req.StreamName != "" {
		query = query.Where("streamPath = ?", req.StreamName)
	}

	// cdnName 不在 HourlyVideoStat 中 → 前端忽略此条件
	var list []models.HourlyVideoStat
	query.Order("statHour").Find(&list)

	c.JSON(200, models.HttpRespMsg{
		Code: 200,
		Msg:  "OK",
		Data: list,
	})
}
