package utils

const (
	APP_NAME = "VLB"
	VERSION  = "v1.0.3"
)

// AppID and AppSecret are loaded from configuration (Secrets) at startup
// via InitSecrets. They MUST NOT be hardcoded here.
var (
	AppID     string
	AppSecret string
)

var versionModification = map[string]string{
	"v1.0.0": "init release",
	"v1.0.1": "multipusher and abrplayer health/playback updates",
	"v1.0.2": "Tencent SRT Opus audio diagnostics",
	"v1.0.3": "Tencent SRT AAC audio compatibility",
}

func GetVersionModification() string {
	return versionModification[VERSION]
}

const (
	ENV_FILE  = ".env"
	CONF_FILE = "conf/vpublisher.yml"

	NODE_CAPACITY_publisher = 100
	CMD_TYPE_startPub       = "COMMAND_TYPE_START_PUB"
	CMD_TYPE_stopPub        = "COMMAND_TYPE_STOP_PUB"
	CMD_TYPE_originDown     = "COMMAND_TYPE_ORIGIN_DOWN"
	CMD_TYPE_originUp       = "COMMAND_TYPE_ORIGIN_UP"
	CMD_TYPE_queryPubPts    = "COMMAND_TYPE_QUERY_PUB_PTS"
	CMD_TYPE_indication     = "COMMAND_TYPE_INDICATION"
	CMD_TYPE_response       = "COMMAND_TYPE_RESPONSE"
)

// InitSecrets injects credentials loaded from the configuration into the
// package-level variables used by Tencent integration helpers. Pass empty
// strings only if you know the corresponding code path is not reachable.
func InitSecrets(appID, appSecret, txKeyMain, txKeyBack string) {
	AppID = appID
	AppSecret = appSecret
	TxKeyMain = txKeyMain
	TxKeyBack = txKeyBack
}
