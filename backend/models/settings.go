package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

const (
	SettingKeyStreamingPlayDomain  = "streamingPlayDomain"
	SettingKeyEnvDomainPrefix      = "envDomain."
	DefaultStreamingPlayDomain     = "play.example.com"
	StreamingPlayDomainDescription = "ApiGetPlayTxUrl playback domain"
)

type EnvDomainSetting struct {
	Env    string `json:"env"`
	Label  string `json:"label"`
	Domain string `json:"domain"`
}

var defaultEnvDomains = []EnvDomainSetting{
	{Env: "local", Label: "local-env", Domain: "http://localhost:8088"},
	{Env: "dev", Label: "UAT-env", Domain: "https://videostat-uat.example.com"},
	{Env: "test", Label: "test-env", Domain: "https://videostat-test.example.com"},
	{Env: "stag", Label: "STAG-env", Domain: "https://videostat-stag.example.com"},
	{Env: "prod", Label: "PROD-env", Domain: "https://videostat-prod.example.com"},
}

type SystemSetting struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement;type:bigint;column:id" json:"id"`
	ConfigKey   string    `gorm:"type:varchar(128);uniqueIndex;column:configKey;not null" json:"configKey"`
	ConfigValue string    `gorm:"type:varchar(512);column:configValue;not null" json:"configValue"`
	Description string    `gorm:"type:varchar(256);column:description" json:"description"`
	CreatedAt   time.Time `gorm:"autoCreateTime;column:createdAt" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime;column:updatedAt" json:"updatedAt"`
}

func (SystemSetting) TableName() string { return "info_system_settings" }

func NormalizePlayDomain(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", fmt.Errorf("play domain required")
	}

	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("invalid play domain: %w", err)
		}
		value = u.Host
	} else {
		value = strings.TrimPrefix(value, "//")
		value = strings.Split(value, "/")[0]
	}

	value = strings.TrimSpace(strings.TrimSuffix(value, "/"))
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("invalid play domain")
	}

	return value, nil
}

func NormalizeAppEnvName(input string) (string, error) {
	env := strings.ToLower(strings.TrimSpace(input))
	if env == "uat" {
		env = "dev"
	}
	for _, item := range defaultEnvDomains {
		if env == item.Env {
			return env, nil
		}
	}
	return "", fmt.Errorf("unsupported APP_ENV: %s", strings.TrimSpace(input))
}

func NormalizePublicBaseURL(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", fmt.Errorf("public base URL required")
	}

	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid public base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("public base URL must start with http:// or https://")
	}
	if u.Host == "" || strings.ContainsAny(u.Host, " \t\r\n") {
		return "", fmt.Errorf("invalid public base URL")
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func envDomainKey(env string) string {
	return SettingKeyEnvDomainPrefix + env
}

func envDomainDescription(item EnvDomainSetting) string {
	return fmt.Sprintf("APP_ENV %s public base URL (%s)", item.Env, item.Label)
}

func defaultStreamingPlayDomain() string {
	if settings.StreamingPlayDomain != "" {
		if value, err := NormalizePlayDomain(settings.StreamingPlayDomain); err == nil {
			return value
		}
	}
	return DefaultStreamingPlayDomain
}

func GetSettingValue(key string, fallback string) string {
	if dbHdl == nil {
		return fallback
	}

	var setting SystemSetting
	if err := dbHdl.Where("configKey = ?", key).First(&setting).Error; err != nil {
		return fallback
	}
	if setting.ConfigValue == "" {
		return fallback
	}
	return setting.ConfigValue
}

func SetSettingValue(key string, value string, description string) error {
	if dbHdl == nil {
		return fmt.Errorf("database not initialized")
	}

	setting := SystemSetting{
		ConfigKey:   key,
		ConfigValue: value,
		Description: description,
	}

	return dbHdl.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "configKey"}},
		DoUpdates: clause.AssignmentColumns([]string{"configValue", "description", "updatedAt"}),
	}).Create(&setting).Error
}

func GetStreamingPlayDomain() string {
	return GetSettingValue(SettingKeyStreamingPlayDomain, defaultStreamingPlayDomain())
}

func SetStreamingPlayDomain(value string) (string, error) {
	domain, err := NormalizePlayDomain(value)
	if err != nil {
		return "", err
	}
	if err := SetSettingValue(SettingKeyStreamingPlayDomain, domain, StreamingPlayDomainDescription); err != nil {
		return "", err
	}
	settings.StreamingPlayDomain = domain
	return domain, nil
}

func ListEnvDomains() []EnvDomainSetting {
	items := make([]EnvDomainSetting, 0, len(defaultEnvDomains))
	for _, item := range defaultEnvDomains {
		domain := GetSettingValue(envDomainKey(item.Env), item.Domain)
		if normalized, err := NormalizePublicBaseURL(domain); err == nil {
			domain = normalized
		} else {
			domain = item.Domain
		}
		items = append(items, EnvDomainSetting{
			Env:    item.Env,
			Label:  item.Label,
			Domain: domain,
		})
	}
	return items
}

func GetEnvDomain(env string) (EnvDomainSetting, error) {
	normalizedEnv, err := NormalizeAppEnvName(env)
	if err != nil {
		return EnvDomainSetting{}, err
	}
	for _, item := range ListEnvDomains() {
		if item.Env == normalizedEnv {
			return item, nil
		}
	}
	return EnvDomainSetting{}, fmt.Errorf("unsupported APP_ENV: %s", normalizedEnv)
}

func SetEnvDomain(env string, value string) (EnvDomainSetting, error) {
	normalizedEnv, err := NormalizeAppEnvName(env)
	if err != nil {
		return EnvDomainSetting{}, err
	}
	domain, err := NormalizePublicBaseURL(value)
	if err != nil {
		return EnvDomainSetting{}, err
	}

	var base EnvDomainSetting
	for _, item := range defaultEnvDomains {
		if item.Env == normalizedEnv {
			base = item
			break
		}
	}
	if base.Env == "" {
		return EnvDomainSetting{}, fmt.Errorf("unsupported APP_ENV: %s", normalizedEnv)
	}

	if err := SetSettingValue(envDomainKey(normalizedEnv), domain, envDomainDescription(base)); err != nil {
		return EnvDomainSetting{}, err
	}
	base.Domain = domain
	return base, nil
}

func SaveStreamingPlayDomainToConfigFile(domain string) error {
	if settings.ConfigFile == "" {
		return fmt.Errorf("config file not loaded")
	}
	return SaveStreamingPlayDomainToConfigFilePath(settings.ConfigFile, domain)
}

func SaveStreamingPlayDomainToConfigFilePath(filename string, domain string) error {
	normalized, err := NormalizePlayDomain(domain)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	config["StreamingPlayDomain"] = normalized
	output, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return err
	}
	output = append(output, '\n')

	return os.WriteFile(filename, output, 0664)
}
