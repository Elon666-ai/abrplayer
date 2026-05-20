package models

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"backend/utils"
)

type LarkConfig struct {
	LarkWebhook string
	LarkSecret  string
}

type MysqlConfig struct {
	Host       string
	BackupHost string
	Port       int
	User       string
	Password   string
	DbName     string
}
type NacosSyncConfig struct {
	HttpPort     int
	HttpsPort    int
	HttpsCrtFile string
	HttpsKeyFile string
	Mysql        MysqlConfig
}

// TencentConfig 腾讯云 push/play 鉴权密钥
type TencentConfig struct {
	TxKeyMain string `json:"txKeyMain"`
	TxKeyBack string `json:"txKeyBack"`
}

// AdminConfig dashboard 登录账号 (开发期明文允许，比较时用 ConstantTimeCompare)
type AdminConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AllConfig struct {
	AppName string
	InstNo  int
	NacosSyncConfig
	LarkApi             LarkConfig
	ApiSecret           string
	StreamingPlayDomain string
	Tencent             TencentConfig `json:"tencent"`
	Admin               AdminConfig   `json:"admin"`
	ConfigFile          string        `json:"-"`
}

var settings = &AllConfig{}

func ConfLoad(filename string) (*AllConfig, error) {

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, settings)
	if err != nil {
		return nil, err
	}
	settings.ConfigFile = filename

	if settings.AppName == "" {
		settings.AppName = utils.APP_NAME
	}

	if settings.ApiSecret == "" {
		settings.ApiSecret = utils.Api_Secret
	}

	apiSecret := os.Getenv("API_SECRET")
	if len(apiSecret) > 1 {
		settings.ApiSecret = apiSecret
	}

	// Admin 凭证 env 覆盖
	if v := os.Getenv("STATAPI_ADMIN_USERNAME"); v != "" {
		settings.Admin.Username = v
	}
	if v := os.Getenv("STATAPI_ADMIN_PASSWORD"); v != "" {
		settings.Admin.Password = v
	}

	aesKey := os.Getenv("BOOTSTRAP_JASYPT_ENCRYPTOR_PASSWORD")
	if aesKey == "" {
		return settings, nil
	}

	config, err := NacosFetchConfig2(aesKey)
	if err != nil {
		fmt.Printf("nacos fetch config failure: %v\n", err)
		// config, err = NacosFetchConfig()
		// if err != nil {
		// 	fmt.Printf("NacosFetchConfig#1 failure: %v\n", err)
		// 	return nil, err
		// }
	} else {
		fmt.Println("nacos fetch config success! LarkWebhook=", config.LarkWebhook)
	}

	settings.ApiSecret = config.ApiSecret
	settings.Mysql.Host = config.MysqlHost
	settings.Mysql.BackupHost = config.BackupMysqlHost
	settings.Mysql.Port, _ = strconv.Atoi(config.MysqlPort)
	settings.Mysql.User = config.MysqlUser
	settings.Mysql.Password = config.MysqlPassword
	settings.Mysql.DbName = config.MysqlDb
	settings.LarkApi.LarkWebhook = config.LarkWebhook
	settings.LarkApi.LarkSecret = config.LarkSecret

	cipherBytes2, err := base64.StdEncoding.DecodeString(config.MysqlPassword)
	if err != nil {
		return nil, fmt.Errorf("decode mysql password base64: %w", err)
	}
	plainBytes, err := utils.Aes128DecryptECB(cipherBytes2, []byte(aesKey))
	if err != nil {
		return nil, fmt.Errorf("decrypt mysql password: %w", err)
	}
	settings.Mysql.Password = string(plainBytes)
	// fmt.Println("Mysql config: ", config.MysqlHost, config.MysqlPort, config.MysqlUser, settings.Mysql.Password, settings.Mysql.DbName)

	return settings, nil
}

func GetAppName() string {
	return fmt.Sprintf("%s%02d", utils.APP_NAME, settings.InstNo)
}
func GetApiSecret() string {
	return settings.ApiSecret
}

// GetAdminUsername / GetAdminPassword 返回 dashboard 登录凭证（可为空，表示未配置）
func GetAdminUsername() string {
	return settings.Admin.Username
}
func GetAdminPassword() string {
	return settings.Admin.Password
}

// GetTencentKeys 返回 push/play 鉴权密钥（来自配置文件）。env 覆盖在 utils.InitTencentKeys 处理。
func GetTencentKeys() (string, string) {
	return settings.Tencent.TxKeyMain, settings.Tencent.TxKeyBack
}

func GetLarkWebhook() string {
	return settings.LarkApi.LarkWebhook
}
func GetLarkSecret() string {
	return settings.LarkApi.LarkSecret
}

func GetLoadedConfigFile() string {
	return settings.ConfigFile
}

