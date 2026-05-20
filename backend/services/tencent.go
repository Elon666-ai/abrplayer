package services

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"strings"
	"time"
	"backend/utils"
)

func GenTxSecret(streamKey string) (string, string) {
	txTime := fmt.Sprintf("%X", time.Now().Add(utils.TX_TOKEN_DURATION*time.Second).Unix())
	hashInput := fmt.Sprintf("%s%s%s", utils.TxKeyBack, streamKey, txTime)
	txSecret := fmt.Sprintf("%x", md5.Sum([]byte(hashInput)))
	return txTime, txSecret
}

func ConvertRTMPURL(inputURL string) (string, error) {
	// Parse the input URL
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %v", err)
	}

	// Extract the scheme (rtmp), host, and path
	scheme := parsedURL.Scheme
	// if scheme != "rtmp" {
	// 	return "", fmt.Errorf("unsupported scheme: %s, expected rtmp", scheme)
	// }
	host := parsedURL.Host
	path := parsedURL.Path

	// Split the path to extract app and stream key
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(pathParts) < 2 {
		return "", fmt.Errorf("invalid streaming path: %s, expected /app/streamkey", path)
	}
	app := pathParts[0]
	streamKey := strings.Join(pathParts[1:], "/")
	txTime, txSecret := GenTxSecret(streamKey)

	// Create query parameters
	query := url.Values{}
	query.Set("txSecret", txSecret)
	query.Set("txTime", txTime)

	// Construct the new URL
	newPath := fmt.Sprintf("/%s?%s/%s", app, query.Encode(), streamKey)
	newURL := fmt.Sprintf("%s://%s%s", scheme, host, newPath)

	return newURL, nil
}
