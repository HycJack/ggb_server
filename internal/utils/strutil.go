package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
)

const DefaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func GenerateRandomString(length int) string {
	return GenerateRandomStringWithCharset(length, DefaultCharset)
}

func GenerateRandomStringWithCharset(length int, charset string) string {
	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		randomIndex, _ := rand.Int(rand.Reader, charsetLen)
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result)
}

func ProcessUrl(imgUrl string, prefix string) string {
	imgPath := imgUrl
	if imgUrl != "" && (strings.Contains(imgUrl, "localhost") || strings.Contains(imgUrl, "127.0.0.1")) {
		path := imgUrl[strings.Index(imgUrl, prefix):]
		rootPath, _ := FindRootPath()
		imgPath = filepath.Join(rootPath, path)
	}
	return imgPath
}

func RecoverUrl(url string, prefix string, static string) string {
	return fmt.Sprintf("%s/%s", prefix, url[strings.Index(url, static):])
}
