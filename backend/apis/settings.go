package apis

import (
	"backend/models"
	"backend/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PlayDomainReq struct {
	Domain string `json:"domain"`
}

type EnvDomainReq struct {
	Env    string `json:"env"`
	Domain string `json:"domain"`
}

func ApiGetPlayDomain(c *gin.Context) {
	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: http.StatusOK,
		Msg:  http.StatusText(http.StatusOK),
		Data: gin.H{"domain": models.GetStreamingPlayDomain()},
	})
}

func ApiPutPlayDomain(c *gin.Context) {
	var req PlayDomainReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid json: "+err.Error(), "")
		return
	}

	domain, err := models.SetStreamingPlayDomain(req.Domain)
	if err != nil {
		sendResponse(c, http.StatusBadRequest, err.Error(), "")
		return
	}
	if err := models.SaveStreamingPlayDomainToConfigFile(domain); err != nil {
		sendResponse(c, http.StatusInternalServerError, "save config file failed: "+err.Error(), "")
		return
	}

	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: http.StatusOK,
		Msg:  http.StatusText(http.StatusOK),
		Data: gin.H{"domain": domain},
	})
}

func ApiGetEnvDomains(c *gin.Context) {
	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: http.StatusOK,
		Msg:  http.StatusText(http.StatusOK),
		Data: gin.H{"items": models.ListEnvDomains()},
	})
}

func ApiPutEnvDomain(c *gin.Context) {
	var req EnvDomainReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid json: "+err.Error(), "")
		return
	}

	item, err := models.SetEnvDomain(req.Env, req.Domain)
	if err != nil {
		sendResponse(c, http.StatusBadRequest, err.Error(), "")
		return
	}

	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: http.StatusOK,
		Msg:  http.StatusText(http.StatusOK),
		Data: item,
	})
}

func ApiGetStatAPIDomain(c *gin.Context) {
	env := c.DefaultQuery("env", utils.GetRunEnv())
	item, err := models.GetEnvDomain(env)
	if err != nil {
		sendResponse(c, http.StatusBadRequest, err.Error(), "")
		return
	}

	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: http.StatusOK,
		Msg:  http.StatusText(http.StatusOK),
		Data: gin.H{
			"env":    item.Env,
			"label":  item.Label,
			"domain": item.Domain,
		},
	})
}
