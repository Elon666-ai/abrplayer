package apis

import (
	"net/http"
	"strconv"
	"time"
	"backend/models"

	"github.com/gin-gonic/gin"
)

// GET /api/stat/dailytrend?days=7&page=1&pageSize=20
func ApiStatDailyTrend(c *gin.Context) {
	db := models.GetDbConn()

	// 参数解析
	daysStr := c.DefaultQuery("days", "7")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")

	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 7
	}
	// 限制查询范围，防止传入过大天数
	if days > 90 {
		days = 90
	}

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 {
		pageSize = 20
	}
	// 限制 pageSize 上限
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize
	// 直接使用 time.Time 做查询参数，避免时区/格式问题
	startTime := time.Now().AddDate(0, 0, -days)

	var (
		results []models.DailyVideoStat
		total   int64
	)

	// 获取总数
	if err := db.Model(&models.DailyVideoStat{}).
		Where("statDate >= ?", startTime).
		Count(&total).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db query failed: "+err.Error(), "")
		return
	}

	// 分页查询（按时间倒序，最近的在前）
	if err := db.
		Where("statDate >= ?", startTime).
		Order("statDate DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&results).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db query failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), models.ListData{
		Items:    results,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// ApiListHourlyVideoStats 分页查询每小时统计结果（按时间倒序）
func ApiListHourlyVideoStats(c *gin.Context) {
	db := models.GetDbConn()

	// 分页参数
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "24")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 24
	}

	// 过滤参数
	streamName := c.Query("streamName")
	startHour := c.Query("startHour")
	endHour := c.Query("endHour")

	query := db.Model(&models.HourlyVideoStat{})

	// 按 streamName 精确过滤
	if streamName != "" {
		query = query.Where("streamPath = ?", streamName)
	}

	// 时间区间过滤（可选）
	if startHour != "" && endHour != "" {
		query = query.Where("statHour BETWEEN ? AND ?", startHour, endHour)
	}

	// 查询总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db count failed: "+err.Error(), "")
		return
	}

	// 查询分页数据
	offset := (page - 1) * pageSize
	var list []models.HourlyVideoStat

	if err := query.
		Order("statHour DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&list).Error; err != nil {

		sendResponse(c, http.StatusInternalServerError, "db query failed: "+err.Error(), "")
		return
	}

	// 返回结构
	resp := models.ListData{
		Items:    list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), resp)
}

func ApiListRoundVideoStats(c *gin.Context) {
	db := models.GetDbConn()
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "20")
	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	query := db.Model(&models.RoundVideoStat{})
	if field := c.Query("fieldName"); field != "" {
		query = query.Where("fieldName = ?", field)
	}

	var total int64
	query.Count(&total)

	var results []models.RoundVideoStat
	query.Order("createdAt DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&results)

	resp := models.ListData{
		Items:    results,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), resp)
}
