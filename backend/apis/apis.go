package apis

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"backend/models"
	"backend/services"
	"backend/tracer"
	"backend/utils"

	"github.com/gin-gonic/gin"
)

func sendResponse(c *gin.Context, code int, codeMsg string, data interface{}) {
	c.JSON(code, models.HttpRespMsg{
		Code: code,
		Msg:  codeMsg,
		Data: data,
	})
}

func ApiStatHealth(c *gin.Context) {
	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{
		"service": "backend",
		"status":  "ok",
		"app":     utils.APP_NAME,
		"version": utils.VERSION,
	})
}

func ApiStatStartPlay(c *gin.Context) {
	var data models.StartPlayStat
	if err := c.ShouldBindJSON(&data); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid request: "+err.Error(), "")
		return
	}
	if data.StreamName == "" {
		sendResponse(c, http.StatusBadRequest, "streamName, gameUserId and gameRound required", "")
		return
	}
	if data.GameRound != "" {
		data.GameRound = "LT" + data.GameRound
	}
	data.CreatedAt = time.Now()
	data.GeoLocation = utils.IPToLocation(c.ClientIP())

	db := models.GetDbConn()
	if err := db.Create(&data).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": data.ID})
}

/*
NOTE: 一次播放过程，分多次调用此接口上报播放时长，后端自行累积总时长。
上报间隔建议为6s一次。腾讯播放https://datacenter.live.qcloud.com/的数据收集间隔为 5s一次。
*/
func ApiStatEndPlay(c *gin.Context) {
	var data models.EndPlayStat
	if err := c.ShouldBindJSON(&data); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid request: "+err.Error(), "")
		return
	}
	if data.StreamName == "" || data.GameUserId == "" || data.GameRound == "" {
		sendResponse(c, http.StatusBadRequest, "streamName, gameUserId and gameRound required", "")
		return
	}
	if data.GameRound != "" {
		data.GameRound = "LT" + data.GameRound
	}
	data.CreatedAt = time.Now()
	data.GeoLocation = utils.IPToLocation(c.ClientIP())

	db := models.GetDbConn()
	if err := db.Create(&data).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": data.ID})
}

func ApiStatLag(c *gin.Context) {
	var data models.PlayLagStat
	if err := c.ShouldBindJSON(&data); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid request: "+err.Error(), "")
		return
	}
	if data.StreamName == "" || data.GameUserId == "" || data.GameRound == "" {
		sendResponse(c, http.StatusBadRequest, "streamName, gameUserId and gameRound required", "")
		return
	}
	if data.GameRound != "" {
		data.GameRound = "LT" + data.GameRound
	}
	data.CreatedAt = time.Now()
	data.GeoLocation = utils.IPToLocation(c.ClientIP())

	db := models.GetDbConn()
	if err := db.Create(&data).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": data.ID})
}

func ApiStatRecord(c *gin.Context) {
	var data models.RecordStat
	if err := c.ShouldBindJSON(&data); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid request: "+err.Error(), "")
		return
	}
	if data.GameRound == "" || data.GameTypeId == "" || data.PlaybackUri == "" {
		sendResponse(c, http.StatusBadRequest, "gameRound, gameTypeId and playbackUri required", "")
		return
	}
	data.CreatedAt = time.Now()

	db := models.GetDbConn()
	if err := db.Create(&data).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": data.ID})
}

func ApiStatPlayback(c *gin.Context) {
	var data models.PlaybackStat
	if err := c.ShouldBindJSON(&data); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid request: "+err.Error(), "")
		return
	}
	if data.GameUserId == "" {
		sendResponse(c, http.StatusBadRequest, "gameUserId required", "")
		return
	}
	data.CreatedAt = time.Now()
	data.GeoLocation = utils.IPToLocation(c.ClientIP())

	db := models.GetDbConn()
	if err := db.Create(&data).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": data.ID})
}

// mapRound：roundId -> fieldName 内存缓存，sync.Map 保证并发安全。
var mapRound sync.Map

func ApiPostFieldRoundStart(c *gin.Context) {
	var req models.FieldRoundStat
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid JSON: "+err.Error(), "")
		return
	}
	if req.RoundId == "" {
		sendResponse(c, http.StatusBadRequest, "roundId required", "")
		return
	}
	req.RoundId = "LT" + req.RoundId

	var item models.FieldRoundStat
	db := models.GetDbConn()
	db.Where(&models.FieldRoundStat{FieldName: req.FieldName, RoundId: req.RoundId}).First(&item)
	if item.ID != 0 {
		sendResponse(c, http.StatusOK, fmt.Sprintf("round:%s already exist!", req.RoundId), "")
		return
	}

	req.RoundStartTime = time.Now()
	req.RoundEndTime = &req.RoundStartTime
	req.Status = "going"
	if err := db.Create(&req).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db insert failed: "+err.Error(), req)
		return
	}

	mapRound.Store(req.RoundId, req.FieldName)
	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": req.ID})
}

func ApiPostFieldRoundEnd(c *gin.Context) {
	var req models.FieldRoundStat
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid JSON: "+err.Error(), "")
		return
	}
	if req.RoundId == "" {
		sendResponse(c, http.StatusBadRequest, "roundId required", "")
		return
	}
	req.RoundId = "LT" + req.RoundId

	if _, ok := mapRound.Load(req.RoundId); !ok {
		sendResponse(c, http.StatusOK, fmt.Sprintf("round:%s not exist!", req.RoundId), "")
		return
	}
	defer mapRound.Delete(req.RoundId)

	lockKey := fmt.Sprintf("round:end:%s:%s", req.FieldName, req.RoundId)
	if !utils.IdempotentLock(lockKey) {
		sendResponse(c, http.StatusOK, "round end already processed", gin.H{"roundId": req.RoundId})
		return
	}
	defer utils.IdempotentUnlock(lockKey)

	var item models.FieldRoundStat
	db := models.GetDbConn()
	db.Where(&models.FieldRoundStat{FieldName: req.FieldName, RoundId: req.RoundId}).First(&item)
	if item.ID == 0 {
		sendResponse(c, http.StatusForbidden, fmt.Sprintf("round:%s not found!", req.RoundId), "")
		return
	}
	tNow := time.Now()
	item.RoundEndTime = &tNow
	item.Status = "done"
	db.Save(&item)
	// 异步生成统计报表，避免接口响应过慢。录像上传有延迟，故延时90秒执行统计。
	tracer.LogDebug(tracer.ID_APP, "sleep 90s to create round:%s report!", item.RoundId)
	time.AfterFunc(90*time.Second, func() {
		services.AggregateRoundStat(item.RoundId, &tNow)
	})

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": item.ID})
}

func ApiPostFieldRoundCancel(c *gin.Context) {
	var req models.FieldRoundStat
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid JSON: "+err.Error(), "")
		return
	}
	if req.RoundId == "" {
		sendResponse(c, http.StatusBadRequest, "roundId required", "")
		return
	}
	req.RoundId = "LT" + req.RoundId
	if _, ok := mapRound.Load(req.RoundId); !ok {
		sendResponse(c, http.StatusOK, fmt.Sprintf("round:%s not exist!", req.RoundId), "")
		return
	}
	defer mapRound.Delete(req.RoundId)

	var item models.FieldRoundStat
	db := models.GetDbConn()
	db.Where(&models.FieldRoundStat{FieldName: req.FieldName, RoundId: req.RoundId}).First(&item)
	if item.ID == 0 {
		sendResponse(c, http.StatusForbidden, fmt.Sprintf("round:%s not found!", req.RoundId), "")
		return
	}
	tNow := time.Now()
	item.RoundEndTime = &tNow
	item.Status = "cancel"
	db.Save(&item)

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), gin.H{"id": item.ID})
}

func ApiListFieldRounds(c *gin.Context) {
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

	query := db.Model(&models.FieldRoundStat{})
	if fieldName := c.Query("fieldName"); fieldName != "" {
		query = query.Where("fieldName = ?", fieldName)
	}
	if roundId := c.Query("roundId"); roundId != "" {
		query = query.Where("roundId = ?", roundId)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db count failed: "+err.Error(), "")
		return
	}

	var results []models.FieldRoundStat
	offset := (page - 1) * pageSize
	if err := query.Order("createdAt DESC").Offset(offset).Limit(pageSize).Find(&results).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "db query failed: "+err.Error(), "")
		return
	}

	resp := models.ListData{
		Items:    results,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), resp)
}

func ApiGetPlayTxUrl(c *gin.Context) {

	stream := c.DefaultQuery("stream", "gsp2w-fwv")
	if strings.Contains(stream, "bps") {
		stream = utils.GetProfileMappedStream(stream)
	}
	txTime, txSecret := services.GenTxSecret(stream)
	txUri := fmt.Sprintf("?txTime=%s&txSecret=%s", txTime, txSecret)
	playDomain := models.GetStreamingPlayDomain()
	uri := "webrtc://" + playDomain + "/live/" + stream + txUri
	// uri := "webrtc://play.example.com/live/" + stream + txUri
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": gin.H{
			"webrtc": uri,
		},
	})
}
