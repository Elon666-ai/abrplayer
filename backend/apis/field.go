package apis

import (
	"net/http"
	"time"
	"backend/models"
	"backend/utils"

	"github.com/gin-gonic/gin"
)

func ApiPostFieldStream(c *gin.Context) {
	var req models.FieldStreamsInfo
	if err := c.ShouldBindJSON(&req); err != nil {
		sendResponse(c, http.StatusBadRequest, "invalid json: "+err.Error(), "")
		return
	}

	db := models.GetDbConn()
	var existing models.FieldStreamsInfo

	// 查询是否已存在相同 fieldName
	result := db.Where("fieldName = ?", req.FieldName).First(&existing)

	if result.Error != nil && result.RowsAffected == 0 {
		// 没有找到 -> 新建
		if err := db.Create(&req).Error; err != nil {
			sendResponse(c, http.StatusInternalServerError, "insert failed: "+err.Error(), "")
			return
		}
		sendResponse(c, http.StatusOK, "created", req)
		return
	}

	if result.Error != nil {
		// 查询出错（不是未找到）
		sendResponse(c, http.StatusInternalServerError, "query failed: "+result.Error.Error(), "")
		return
	}

	// 找到了 -> 更新
	existing.StreamPath = req.StreamPath
	existing.Description = req.Description
	existing.UpdatedAt = time.Now()

	if err := db.Save(&existing).Error; err != nil {
		sendResponse(c, http.StatusInternalServerError, "update failed: "+err.Error(), "")
		return
	}

	sendResponse(c, http.StatusOK, "updated", existing)
}

// func ApiListFieldStreams(c *gin.Context) {
// 	db := models.GetDbConn()

// 	keyword := c.Query("keyword") // 可选搜索
// 	var results []models.FieldStreamsInfo
// 	query := db.Model(&models.FieldStreamsInfo{})
// 	if keyword != "" {
// 		query = query.Where("fieldName LIKE ?", "%"+keyword+"%")
// 	}

// 	if err := query.Order("fieldName ASC").Find(&results).Error; err != nil {
// 		sendResponse(c, http.StatusInternalServerError, "db query failed: "+err.Error(), "")
// 		return
// 	}

// 	sendResponse(c, http.StatusOK, http.StatusText(http.StatusOK), results)
// }

// 返回 FieldName -> []StreamPath 的映射对象，供前端下拉使用
func ApiListFieldStreams(c *gin.Context) {
	db := models.GetDbConn()

	var items []models.FieldStreamsInfo
	if err := db.Order("fieldName ASC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.HttpRespMsg{
			Code: 500,
			Msg:  "db query failed: " + err.Error(),
			Data: nil,
		})
		return
	}

	// 构建 mapping: fieldName -> []streamPath
	result := make(map[string][]string)
	for _, it := range items {
		if it.FieldName == "" {
			continue
		}
		// append streamPath if non-empty and avoid duplicates
		if it.StreamPath == "" {
			continue
		}
		arr := result[it.FieldName]
		// optional: avoid duplicate entries
		duplicate := false
		for _, v := range arr {
			if v == it.StreamPath {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result[it.FieldName] = append(arr, utils.ExtractStreamPath(it.StreamPath))
		}
	}

	c.JSON(http.StatusOK, models.HttpRespMsg{
		Code: 200,
		Msg:  "OK",
		Data: result,
	})
}

func ApiDeleteFieldStream(c *gin.Context) {
	fieldName := c.Param("fieldName")
	if fieldName == "" {
		sendResponse(c, http.StatusBadRequest, "missing fieldName", "")
		return
	}

	db := models.GetDbConn()
	result := db.Where("fieldName = ?", fieldName).Delete(&models.FieldStreamsInfo{})
	if result.Error != nil {
		sendResponse(c, http.StatusInternalServerError, "delete failed: "+result.Error.Error(), "")
		return
	}

	if result.RowsAffected == 0 {
		sendResponse(c, http.StatusNotFound, "not found", "")
		return
	}

	sendResponse(c, http.StatusOK, "deleted", "")
}

func ApiFieldsList(c *gin.Context) {
	db := models.GetDbConn()

	var f1 []models.FieldsInfo
	var f2 []models.FieldStreamsInfo

	fieldSet := make(map[string]bool)

	// 读取 FieldsInfo
	if err := db.Find(&f1).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.HttpRespMsg{
			Code: 500,
			Msg:  "db query failed: " + err.Error(),
		})
		return
	}

	// 读取 FieldStreamsInfo
	if err := db.Find(&f2).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.HttpRespMsg{
			Code: 500,
			Msg:  "db query failed: " + err.Error(),
		})
		return
	}

	// 统一去重
	for _, f := range f1 {
		if f.FieldName != "" {
			fieldSet[f.FieldName] = true
		}
	}
	for _, f := range f2 {
		if f.FieldName != "" {
			fieldSet[f.FieldName] = true
		}
	}

	// 转为列表
	fields := []string{}
	for name := range fieldSet {
		fields = append(fields, name)
	}

	c.JSON(200, models.HttpRespMsg{
		Code: 200,
		Msg:  "OK",
		Data: fields,
	})
}
