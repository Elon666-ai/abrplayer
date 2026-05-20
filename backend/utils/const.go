package utils

import "os"

const (
	APP_NAME   = "abrplayer-backend"
	VERSION    = "v1.1.2"
	Api_Secret = ""
)

var versionModification = map[string]string{
	"v1.1.2": "1. expose statapi domain for abrplayer reporting.",
	"v1.1.1": "1. add APP_ENV public base URL settings.",
	"v1.1.0": "1. add 'LT' as round prefix.",
	"v1.0.9": "1. fix get cpu/memory wrong issue.",
	"v1.0.8": `1. 新增api: /api/play/txUrl GET 2. add a new field:canHevc`,
	"v1.0.7": `1. api增加gamtypeId字段. 2. 增加view details url.`,
	"v1.0.6": `1. 支持api: /api/stat/round/cancel.`,
	"v1.0.5": `1. 支持dashboard页面.`,
	"v1.0.4": `1. stream 状态 active → broken 连续两次 ffprobe 失败 才触发.`,
	"v1.0.3": `1. 查表统计录像时长采用模糊查表.`,
	"v1.0.2": `1. 支持nacos settings. 2. support decrypt mysql password from env or nacos.`,
	"v1.0.1": `1. 增加.env支持. 2. use $env to load different config files.`,
	"v1.0.0": `init release`,
}

func GetVersionModification() string {
	return versionModification[VERSION]
}

const (
	CONF_FILE     = "conf/backend.%s.json"
	RESET_LOGFILE = "./resetlog.txt"
	PID_LOGFILE   = "./pid.txt"
	ENV_FILE      = ".env"

	TX_TOKEN_DURATION = 86400
)

// 腾讯云 push/play 鉴权密钥；启动时由 InitTencentKeys 注入。
var (
	TxKeyMain string
	TxKeyBack string
)

// InitTencentKeys 在加载配置后调用，将 conf+env 提供的密钥写入运行时变量。
// 环境变量优先于配置文件。
func InitTencentKeys(main, back string) {
	if v := os.Getenv("STATAPI_TX_KEY_MAIN"); v != "" {
		main = v
	}
	if v := os.Getenv("STATAPI_TX_KEY_BACK"); v != "" {
		back = v
	}
	TxKeyMain = main
	TxKeyBack = back
}

// 流状态常量定义
const (
	StateActive    = "active"    // 推流中
	StateBroken    = "broken"    // 中断中
	StateRecovered = "recovered" // 恢复中
	StateEnded     = "ended"     // 已结束
)

// 告警触发
const (
	AlarmStreamBroken      = "streamBroken"
	AlarmPlayTimeout       = "playTimeout"
	AlarmRecordNotComplete = "recordNotComplete"
	ReportPerRound         = "reportPerRound"
)

/*
https://videostat-test.example.com/api/play/txUrl?stream=1080p_hevc_1mbps
*/
func GetStatDetailUrl() string {
	switch GetRunEnv() {
	case "local":
		return "http://localhost:8088/dashboard/"
	case "test":
		return "https://videostat-test.example.com/dashboard/"
	case "dev", "uat":
		return "https://videostat-uat.example.com/dashboard/"
	case "stag":
		return "https://videostat-stag.example.com/dashboard/"
	case "prod":
		return "https://videostat-prod.example.com/dashboard/"
	default:
		return "http://localhost:8088/dashboard/"
	}
}

func GetProfileMappedStream(profile string) string {

	switch profile {
	case "1080p_hevc_1mbps":
		return "gsp2w-fwv-hd5"
	case "720p_hevc_400kbps":
		return "gsp2w-fwv-sd5"
	case "1080p_h264_2mbps":
		return "gsp2w-fwv-hd4"
	case "720p_h264_1mbps":
		return "gsp2w-fwv-sd4"
	case "540p_h264_400kbps":
		return "gsp2w-fwv-ld4"
	case "audio_64kbps":
		return "gsp2w-audio"
	}

	return ""
}
