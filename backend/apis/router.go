package apis

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"

	"backend/tracer"
	"backend/utils"
)

func PollHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": gin.H{"app": utils.APP_NAME, "version": utils.VERSION},
	})
}

// allowedOrigins 根据环境返回 CORS 白名单
func allowedOrigins(env string) []string {
	switch env {
	case "local":
		return []string{"http://localhost:8088", "http://127.0.0.1:8088"}
	case "test":
		return []string{"https://videostat-test.example.com"}
	case "dev", "uat":
		return []string{"https://videostat-uat.example.com"}
	case "stag":
		return []string{"https://videostat-stag.example.com"}
	case "prod":
		return []string{"https://videostat-prod.example.com"}
	default:
		return []string{"http://localhost:8088", "http://127.0.0.1:8088"}
	}
}

// loadSessionSecret 读取 STATAPI_SESSION_SECRET，未设置时生成随机 32 字节并 log warning
func loadSessionSecret() []byte {
	if v := os.Getenv("STATAPI_SESSION_SECRET"); v != "" {
		return []byte(v)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		tracer.LogWarn(tracer.ID_APP, "rand.Read failed: %v, fallback to APP_NAME", err)
		return []byte(utils.APP_NAME)
	}
	tracer.LogWarn(tracer.ID_APP, "STATAPI_SESSION_SECRET not set; using ephemeral random secret (sessions will NOT survive restart)")
	return buf
}

/*
ratelimit:
每个 IP（即 c.ClientIP()）,每分钟最多 1000 次请求,超过则返回 HTTP 429.
对所有上报 API 生效（startplay / endplay / lag / record）
*/
func NewRouter(env string) *gin.Engine {

	gin.SetMode("release")

	//Default returns an Engine instance with the Logger and Recovery middleware already attached.
	r := gin.Default()
	// 信任 loopback proxy 以便 ClientIP() 在反向代理后仍能正确取到真实 IP；
	// 其他来源的 X-Forwarded-* 将被忽略。
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	origins := allowedOrigins(env)
	r.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"POST", "OPTIONS", "GET", "PUT", "DELETE"},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * 60 * 60,
	}))

	// Session store with HttpOnly/Secure/SameSite + proper secret
	store := cookie.NewStore(loadSessionSecret())
	secureCookie := env != "local"
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   60 * 60 * 8, // 8h
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions(utils.APP_NAME, store))

	if adminWebDir := resolveAdminWebDir(); adminWebDir != "" {
		r.Static("/dashboard", adminWebDir)
	} else {
		tracer.LogWarn(tracer.ID_APP, "no admin-web loaded!")
	}

	// abrplayer static assets — serve at /abrplayer. ABRPLAYER_ROOT env override
	// is honoured by resolveAbrPlayerDir; falls back to ../abrplayer when unset.
	abrPlayerDir := resolveAbrPlayerDir()
	if abrPlayerDir != "" {
		r.Static("/abrplayer", abrPlayerDir)
	} else {
		tracer.LogWarn(tracer.ID_APP, "no abrplayer static dir loaded!")
	}

	v1 := r.Group("/")
	{
		v1.GET("healthz", PollHealth)
		v1.GET("api/stat/health", ApiStatHealth)
		v1.GET("api/settings/statapi-domain", ApiGetStatAPIDomain)
		// bandwidth probe: no auth, but rate-limited
		v1.GET("bandwidth-probe", utils.RateLimitMiddleware(), BandwidthProbe)
		v1.HEAD("bandwidth-probe", utils.RateLimitMiddleware(), BandwidthProbe)
	}

	// auth 路由：login/logout/me 公开（带限流）
	authApi := r.Group("/api/auth/", utils.RateLimitMiddleware())
	{
		authApi.POST("login", ApiAuthLogin)
		authApi.POST("logout", ApiAuthLogout)
		authApi.GET("me", ApiAuthMe)
	}

	listApi := r.Group("/list/", utils.RateLimitMiddleware(), AuthMiddleware())
	{
		listApi.GET("fields", ApiFieldsList)
		listApi.GET("fields/streams", ApiListFieldStreams)
		listApi.GET("hourly", ApiListHourlyVideoStats)
		listApi.GET("daily", ApiStatDailyTrend)
		listApi.GET("fieldround", ApiListFieldRounds)
		listApi.GET("round", ApiListRoundVideoStats)
	}
	// stat 上报端点：player / recorder 无需 session，但保留限流。
	// 注意路径拼接：Group("/api/stat/") + "play/start" => /api/stat/play/start。
	statIngest := r.Group("/api/stat/", utils.RateLimitMiddleware(), RequestLoggerMiddleware())
	{
		statIngest.POST("play/start", ApiStatStartPlay)
		statIngest.POST("play/end", ApiStatEndPlay)
		statIngest.POST("lag", ApiStatLag)
		statIngest.POST("record", ApiStatRecord)
		statIngest.POST("playback", ApiStatPlayback)
	}

	// 建议先限流再鉴权：utils.RateLimitMiddleware() 放前面可先拦截大量无效请求
	api := r.Group("/api/", utils.RateLimitMiddleware(), AuthMiddleware(), RequestLoggerMiddleware())
	{
		api.POST("/fields/stream", ApiPostFieldStream)
		api.DELETE("/fields/stream/:fieldName", ApiDeleteFieldStream)
		api.GET("play/txUrl", ApiGetPlayTxUrl)
		api.GET("settings/play-domain", ApiGetPlayDomain)
		api.PUT("settings/play-domain", ApiPutPlayDomain)
		api.GET("settings/env-domains", ApiGetEnvDomains)
		api.PUT("settings/env-domains", ApiPutEnvDomain)

		api.POST("stat/round/start", ApiPostFieldRoundStart)
		api.POST("stat/round/end", ApiPostFieldRoundEnd)
		api.POST("stat/round/cancel", ApiPostFieldRoundCancel)

		api.POST("/stats/play/list", ApiStatsPlayList)
		api.POST("/stats/play/trend", ApiStatsPlayTrend)
	}
	test := r.Group("/test/")
	{
		test.GET("report/round", TestGetRoundReport)
		test.GET("report/hourly", TestGetHourlyReport)
		test.GET("report/daily", TestGetDailyReport)

	}

	// NoRoute fallback: any unmatched non-API path falls through to abrplayer
	// static assets so the player can be opened at "/" or any sub-path.
	if abrPlayerDir != "" {
		rootAbs, _ := filepath.Abs(abrPlayerDir)
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/list/") ||
				strings.HasPrefix(p, "/dashboard/") || strings.HasPrefix(p, "/test/") ||
				strings.HasPrefix(p, "/abrplayer/") {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			if p == "/" {
				c.Redirect(http.StatusFound, "/abrplayer/index.html")
				return
			}
			rel := strings.TrimPrefix(p, "/")
			fullPath := filepath.Join(abrPlayerDir, rel)
			// guard against path traversal: must remain inside abrPlayerDir
			abs, err := filepath.Abs(fullPath)
			if err != nil || !strings.HasPrefix(abs, rootAbs) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			if !utils.CheckFileExist(fullPath) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.File(fullPath)
		})
	}

	return r
}

// resolveAbrPlayerDir returns the abrplayer static root, honouring the
// ABRPLAYER_ROOT env var and falling back to a list of candidate paths so
// the binary can run from `backend/`, the repo root, or the container.
func resolveAbrPlayerDir() string {
	candidates := []string{}
	if v := strings.TrimSpace(os.Getenv("ABRPLAYER_ROOT")); v != "" {
		candidates = append(candidates, v)
	}
	candidates = append(candidates,
		"./abrplayer",
		"../abrplayer",
		"../../abrplayer",
		"/opt/abrplayer",
	)
	for _, dir := range candidates {
		indexPath := filepath.Join(dir, "index.html")
		if utils.CheckFileExist(indexPath) {
			return dir
		}
	}
	return ""
}

func resolveAdminWebDir() string {
	candidates := []string{
		"./admin-web",
		"../admin-web",
		"../../admin-web",
		"./web",
		"../web",
	}
	for _, dir := range candidates {
		indexPath := filepath.Join(dir, "index.html")
		if utils.CheckFileExist(indexPath) {
			return dir
		}
	}
	return ""
}

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 读取 body 副本
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // 恢复body供后续读取

		if len(bodyBytes) > 1024 {
			bodyBytes = bodyBytes[:1024]
		}
		// 打印请求信息
		tracer.LogDebug(tracer.ID_APP, "[REQUEST] %s %s | Body: %s", c.Request.Method, c.Request.URL.Path, string(bodyBytes))

		c.Next()

		// 响应日志
		tracer.LogDebug(tracer.ID_APP, "[RESPONSE] %s %s | Status: %d | Cost: %v",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}
