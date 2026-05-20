package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"encoding/hex"
	"errors"
)

func MD5Sum(str string) string {
	s := md5.New()
	s.Write([]byte(str))
	return hex.EncodeToString(s.Sum(nil))
}

// -------- PKCS7 填充/去除 --------
func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func pkcs7Unpadding(data []byte) []byte {
	length := len(data)
	unpadding := int(data[length-1])
	return data[:(length - unpadding)]
}

// -------- 自动补齐 key --------
func normalizeKey(key []byte) []byte {
	if len(key) >= 16 {
		return key[:16]
	}
	newKey := make([]byte, 16)
	copy(newKey, key) // 自动补 0x00
	return newKey
}

// -------- ECB 模式实现 --------
func Aes128EncryptECB(src, key []byte) ([]byte, error) {
	key = normalizeKey(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	src = pkcs7Padding(src, bs)

	encrypted := make([]byte, len(src))
	for start := 0; start < len(src); start += bs {
		block.Encrypt(encrypted[start:start+bs], src[start:start+bs])
	}
	return encrypted, nil
}

func Aes128DecryptECB(src, key []byte) ([]byte, error) {
	key = normalizeKey(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	if len(src) == 0 || len(src)%bs != 0 {
		return nil, errors.New("aes128: invalid ciphertext length")
	}

	decrypted := make([]byte, len(src))
	for start := 0; start < len(src); start += bs {
		block.Decrypt(decrypted[start:start+bs], src[start:start+bs])
	}
	decrypted = pkcs7Unpadding(decrypted)
	return decrypted, nil
}
