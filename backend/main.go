// main executable.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"backend/apis"
	"backend/models"
	"backend/services"
	"backend/tasks"
	"backend/tracer"
	"backend/utils"

	_ "go.uber.org/automaxprocs"
)

var BuildTime string

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "-V" || os.Args[1] == "-v") {
		fmt.Println(utils.APP_NAME, utils.VERSION, "BuildTime:", BuildTime, utils.GetVersionModification())
		return
	}

	defer tracer.TryException()
	utils.AppendResetLog()

	confFile, env := utils.GetConfigFile()
	fmt.Printf("env=%s, config=%s\n", env, confFile)
	if !utils.CheckFileExist(confFile) {
		tracer.LogWarn(tracer.ID_APP, "%s not found!", confFile)
		os.Exit(1)
	}
	conf, err := models.ConfLoad(confFile)
	if err != nil {
		tracer.LogWarn(tracer.ID_APP, "load %s failure! Exit! err=%v", confFile, err)
		os.Exit(2)
	}

	// 初始化腾讯云密钥（env 覆盖在 InitTencentKeys 内处理）
	txMain, txBack := models.GetTencentKeys()
	utils.InitTencentKeys(txMain, txBack)
	if utils.TxKeyMain == "" || utils.TxKeyBack == "" {
		tracer.LogWarn(tracer.ID_APP, "tencent keys not configured (tencent.txKeyMain/txKeyBack or env STATAPI_TX_KEY_MAIN/BACK)")
	}

	numCPU := runtime.GOMAXPROCS(0)
	capacity, err := utils.GetContainerMemoryLimit()
	if err != nil {
		capacity = ((2 << 10) << 10) << 10
	}
	debug.SetMemoryLimit(int64(capacity * 75 / 100))
	debug.SetGCPercent(200)

	appName := models.GetAppName()
	strBuildInfo := fmt.Sprintf("%s[%s], version=%s, BuildTime=%s, cpunum=%d, memory=%dGB", appName, env, utils.VERSION, BuildTime, numCPU, capacity/1024/1024/1024)
	tracer.LogInfo(tracer.ID_APP, "%v", strBuildInfo)

	err = models.InitDbConn()
	if err == nil {
		tracer.LogInfo(tracer.ID_APP, "init mysql success!")
		defer models.CloseDbConn()
	} else {
		tracer.LogWarn(tracer.ID_APP, "mysql-init failure! error=%v", err)
		os.Exit(4)
	}
	tracer.LogInfo(tracer.ID_APP, "Load studio config data!")
	services.LoadFieldStreamMapping()

	err = utils.InitGeoIP("./data/GeoLite2-City.mmdb")
	if err != nil {
		tracer.LogWarn(tracer.ID_APP, "load ip2geo library failure! error=%v", err)
	} else {
		tracer.LogDebug(tracer.ID_APP, "load ip2geo library into RAM success!")
	}

	go tasks.MonitorMainThread()

	go tasks.ScheduleDailyStat()

	if conf.HttpPort > 1000 {
		go func() {
			defer tracer.TryException()

			r := apis.NewRouter(env)
			tracer.LogInfo(tracer.ID_APP, "restApiSvr listening on http::%d", conf.HttpPort)
			r.Run(fmt.Sprintf(":%d", conf.HttpPort))
		}()
	}

	if conf.HttpsPort > 1000 {
		go func() {
			defer tracer.TryException()

			r := apis.NewRouter(env)
			_, err1 := os.Stat(conf.HttpsCrtFile)
			_, err2 := os.Stat(conf.HttpsKeyFile)
			if err1 == nil && err2 == nil {
				tracer.LogInfo(tracer.ID_APP, "restApiSvr listening on https::%d", conf.HttpsPort)
				r.RunTLS(fmt.Sprintf(":%d", conf.HttpsPort), conf.HttpsCrtFile, conf.HttpsKeyFile)
			} else {
				tracer.LogWarn(tracer.ID_APP, "%v,%v", err1, err2)
			}
		}()
	}
	tracer.LogInfo(tracer.ID_APP, "restApiSvr ratelimit is qps=%d", utils.MaxRequestsPerSecond)
	tracer.LogInfo(tracer.ID_APP, "enter mainThread env=%s, config=%s", env, confFile)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	tracer.LogInfo(tracer.ID_APP, "exit %s env=%s, config=%s", utils.APP_NAME, env, confFile)
}
