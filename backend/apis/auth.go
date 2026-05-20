package apis

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"backend/models"
	"backend/utils"
)

const sessionUserKey = "user"

// loginReq dashboard login payload
type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ApiAuthLogin POST /api/auth/login
// 成功：HTTP 200 + 写入 session cookie + {"code":0,"msg":"ok","data":{"username":...}}
// 失败：HTTP 200 + {"code":401,"msg":"invalid credentials"} （故意不用 401 防止浏览器弹框）
func ApiAuthLogin(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "msg": "invalid json: " + err.Error()})
		return
	}

	wantUser := models.GetAdminUsername()
	wantPass := models.GetAdminPassword()
	if wantUser == "" || wantPass == "" {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "admin not configured"})
		return
	}

	// 用 ConstantTimeCompare 防 timing attack
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(wantUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(wantPass)) == 1
	if !(userOK && passOK) {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "invalid credentials"})
		return
	}

	sess := sessions.Default(c)
	sess.Set(sessionUserKey, req.Username)
	if err := sess.Save(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "msg": "session save failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ok",
		"data": gin.H{"username": req.Username},
	})
}

// ApiAuthLogout POST /api/auth/logout
func ApiAuthLogout(c *gin.Context) {
	sess := sessions.Default(c)
	sess.Clear()
	sess.Options(sessions.Options{
		Path:   "/",
		MaxAge: -1,
	})
	_ = sess.Save()
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
}

// ApiAuthMe GET /api/auth/me
func ApiAuthMe(c *gin.Context) {
	sess := sessions.Default(c)
	v := sess.Get(sessionUserKey)
	if v == nil {
		c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "unauthenticated"})
		return
	}
	username, _ := v.(string)
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"username": username},
	})
}

// AuthMiddleware 优先 session cookie，回退 Bearer Token
// 本机访问仅在 env=local 时放行。
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. session cookie 认证
		sess := sessions.Default(c)
		if v := sess.Get(sessionUserKey); v != nil {
			if username, ok := v.(string); ok && username != "" {
				c.Set("user", username)
				c.Next()
				return
			}
		}

		// 2. Bearer Token 回退（保留 player 端兼容性）
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			expected := models.GetApiSecret()
			if expected != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 {
				c.Next()
				return
			}
		}

		// 3. 仅 local 环境放行 127.0.0.1（便于本地调试）
		if utils.GetRunEnv() == "local" {
			clientIP := c.ClientIP()
			if clientIP == "127.0.0.1" || clientIP == "::1" {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"code": http.StatusUnauthorized,
			"msg":  "unauthenticated",
		})
		c.Abort()
	}
}
