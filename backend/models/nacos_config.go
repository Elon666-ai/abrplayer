package models

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"backend/utils"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// Config 配置结构体
type NacosConfig struct {
	ApiSecret       string
	MysqlHost       string
	MysqlPort       string
	MysqlUser       string
	MysqlPassword   string
	MysqlDb         string
	BackupMysqlHost string
	LarkWebhook     string
	LarkSecret      string
}

// fetchNacosConfig 从 Nacos 获取配置
func NacosFetchConfig2(aesKey string) (*NacosConfig, error) {
	// 从环境变量读取 Nacos 连接信息
	nacosServer := os.Getenv("BOOTSTRAP_NACOS_CONFIG_SERVER")
	nacosPassword := os.Getenv("BOOTSTRAP_NACOS_CONFIG_PASSWORD")
	nacosUsername := os.Getenv("BOOTSTRAP_NACOS_CONFIG_USERNAME")
	nacosNamespace := os.Getenv("BOOTSTRAP_NACOS_CONFIG_NAMESPACE")
	nacosPortStr := os.Getenv("BOOTSTRAP_NACOS_CONFIG_SERVER_PORT")

	cipherBytes2, err := base64.StdEncoding.DecodeString(nacosPassword)
	if err != nil {
		return nil, fmt.Errorf("decode nacos password base64: %w", err)
	}
	plainBytes, err := utils.Aes128DecryptECB(cipherBytes2, []byte(aesKey))
	if err != nil {
		return nil, fmt.Errorf("decrypt nacos password: %w", err)
	}
	nacosPassword = string(plainBytes)

	// 验证必要的环境变量
	if nacosServer == "" {
		return nil, fmt.Errorf("环境变量 BOOTSTRAP_NACOS_CONFIG_SERVER 未设置")
	}
	if nacosPortStr == "" {
		nacosPortStr = "8848" // 默认端口
	}

	nacosPort, err := strconv.ParseUint(nacosPortStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("无效的端口号: %v", err)
	}

	// 创建 Nacos 客户端配置
	serverConfigs := []constant.ServerConfig{
		{
			IpAddr: nacosServer,
			Port:   nacosPort,
		},
	}

	clientConfig := constant.ClientConfig{
		NamespaceId:         nacosNamespace,
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "./logs",
		CacheDir:            "./cache",
		LogLevel:            "info",
	}

	// 如果提供了用户名和密码,添加到配置中
	if nacosUsername != "" {
		clientConfig.Username = nacosUsername
	}
	if nacosPassword != "" {
		clientConfig.Password = nacosPassword
	}

	// 创建配置客户端
	configClient, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Nacos 客户端失败: %v", err)
	}

	// 获取配置
	// kept for backward compatibility with existing nacos config
	dataId := "statapi"
	group := "DEFAULT_GROUP"

	content, err := configClient.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return nil, fmt.Errorf("获取配置失败: %v", err)
	}

	if content == "" {
		return nil, fmt.Errorf("配置内容为空")
	}

	// 解析配置内容
	config, err := parseConfig(content)
	if err != nil {
		return nil, fmt.Errorf("解析配置失败: %v", err)
	}

	return config, nil
}

// parseConfig 解析配置内容
func parseConfig(content string) (*NacosConfig, error) {
	config := &NacosConfig{}
	lines := strings.Split(content, "\n")

	configMap := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			configMap[key] = value
		}
	}

	// 映射配置项
	config.ApiSecret = configMap["ApiSecret"]
	config.MysqlHost = configMap["MysqlHost"]
	config.MysqlPort = configMap["MysqlPort"]
	config.MysqlUser = configMap["MysqlUser"]
	config.MysqlPassword = configMap["MysqlPassword"]
	config.MysqlDb = configMap["MysqlDb"]
	config.BackupMysqlHost = configMap["BackupMysqlHost"]
	config.LarkWebhook = configMap["LarkWebhook"]
	config.LarkSecret = configMap["LarkSecret"]

	return config, nil
}

// 可选: 监听配置变化
