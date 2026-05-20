package apis

import (
	"net/http"
	"backend/models"
	"backend/services"

	"github.com/gin-gonic/gin"
)

func TestGetRoundReport(c *gin.Context) {

	round := c.Query("round")
	go services.AggregateRoundStat(round, nil)

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), "")
}

func TestGetHourlyReport(c *gin.Context) {
	db := models.GetDbConn()
	go services.AggregateHourlyStat(db)

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), "")
}

func TestGetDailyReport(c *gin.Context) {
	db := models.GetDbConn()
	go services.AggregateDailyStat(db)

	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), "")
}
