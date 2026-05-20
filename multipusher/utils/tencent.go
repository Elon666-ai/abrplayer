package utils

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// 注意：推流和拉流采用不同的key
// TxKeyMain / TxKeyBack are loaded from configuration (Secrets) at startup via
// InitSecrets in const.go. They MUST NOT be hardcoded here.
const (
	TX_PUBISH_TOKEN_DAYS = 30
)

var (
	TxKeyMain string
	TxKeyBack string
)

func GenTencentTxSecret(streamKey string) (string, string) {
	return GenTencentTxSecretWithDays(streamKey, TX_PUBISH_TOKEN_DAYS)
}

func GenTencentTxSecretWithDays(streamKey string, tokenDays int) (string, string) {
	if tokenDays <= 0 {
		tokenDays = TX_PUBISH_TOKEN_DAYS
	}
	txTime := fmt.Sprintf("%X", time.Now().Add(time.Duration(tokenDays)*24*time.Hour).Unix())
	hashInput := fmt.Sprintf("%s%s%s", TxKeyMain, streamKey, txTime)
	txSecret := fmt.Sprintf("%x", md5.Sum([]byte(hashInput)))
	return txTime, txSecret
}

func BuildTencentSRTURL(host string, port int, app string, streamKey string, tokenDays int) string {
	host = strings.TrimSpace(host)
	app = strings.Trim(strings.TrimSpace(app), "/")
	streamKey = strings.TrimSpace(streamKey)
	txTime, txSecret := GenTencentTxSecretWithDays(streamKey, tokenDays)
	return fmt.Sprintf("srt://%s:%d?streamid=#!::h=%s,r=%s/%s,txSecret=%s,txTime=%s",
		host, port, host, app, streamKey, txSecret, txTime)
}

// ---------------------
// 解析 r=live/gsp2w-fwv-ld4 得到 streamKey = gsp2w-fwv-ld4
// ---------------------
func ExtractStreamKey(rValue string) string {
	// r=live/xxxx
	if !strings.Contains(rValue, "/") {
		return rValue
	}
	parts := strings.Split(rValue, "/")
	return parts[len(parts)-1]
}

func UpdateServiceJsonTxSecret(servicePath string) error {
	// 读取原始 json（保持原文，不格式化）
	raw, err := os.ReadFile(servicePath)
	if err != nil {
		return err
	}

	// 解析 json（获得结构，但我们最终不 Marshal）
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}

	settings := obj["settings"].(map[string]interface{})
	server := settings["server"].(string)
	keyStr := settings["key"].(string)

	// --------------------------
	// 2. 提取 streamKey
	// --------------------------
	streamKey := ""
	if idx := strings.Index(server, "r=live/"); idx > 0 {
		remain := server[idx+len("r=live/"):]
		end := strings.IndexAny(remain, ",&")
		if end == -1 {
			streamKey = remain
		} else {
			streamKey = remain[:end]
		}
	}
	if streamKey == "" {
		return fmt.Errorf("cannot extract streamKey from server")
	}

	// --------------------------
	// 3. 生成新的 txSecret / txTime
	// --------------------------
	txTime, txSecret := GenTencentTxSecret(streamKey)

	// --------------------------
	// 4. 替换 txSecret / txTime
	// --------------------------
	server = replaceParam(server, "txSecret", txSecret)
	server = replaceParam(server, "txTime", txTime)

	keyStr = replaceParam(keyStr, "txSecret", txSecret)
	keyStr = replaceParam(keyStr, "txTime", txTime)

	// --------------------------
	// 5. 用 json.Marshal 构建 JSON（OBS 接受任意字段顺序）
	// --------------------------
	outObj := map[string]interface{}{
		"type": obj["type"].(string),
		"settings": map[string]interface{}{
			"server":   server,
			"use_auth": false,
			"bwtest":   false,
			"key":      keyStr,
		},
	}
	outBytes, err := json.Marshal(outObj)
	if err != nil {
		return err
	}

	// 写回，不做格式化
	return os.WriteFile(servicePath, outBytes, 0644)
}

// ---------------------------
// 替换 k=v 格式的参数值
// ---------------------------
func replaceParam(s, param, val string) string {
	tag := param + "="
	idx := strings.Index(s, tag)
	if idx == -1 {
		return s
	}
	start := idx + len(tag)
	end := start
	for end < len(s) && !strings.ContainsRune(",&", rune(s[end])) {
		end++
	}
	return s[:start] + val + s[end:]
}
