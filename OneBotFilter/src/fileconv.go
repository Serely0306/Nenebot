package onebotfilter

import (
	"fmt"
	"log"
	"strings"
)

// ConvertFileToURL 将消息中的 file:// 路径转换为 HTTP URL
// 使用简单字符串替换，避免正则在多字节UTF-8路径中的问题
func ConvertFileToURL(msgData []byte) []byte {
	cfg := CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" || cfg.PublicURL == "" {
		return msgData
	}

	root := cfg.Root
	// 确保root不以/结尾，避免双斜杠
	root = strings.TrimRight(root, "/")
	publicURL := strings.TrimRight(cfg.PublicURL, "/")
	token := CONFIG.Server.AccessToken

	// 构建替换目标
	var httpPrefix string
	if token != "" {
		httpPrefix = fmt.Sprintf("%s/files/%s", publicURL, token)
	} else {
		httpPrefix = fmt.Sprintf("%s/files", publicURL)
	}

	msg := string(msgData)
	changed := false

	// 0. 归一化: file:////+ → file:/// (修复 ZeroBot 中 file.BOTPATH 绝对路径导致的多余斜杠)
	for strings.Contains(msg, "file:////") {
		msg = strings.ReplaceAll(msg, "file:////", "file:///")
		changed = true
	}

	// 1. 替换 file:// 前缀的路径
	// file:///root/bot/xxx -> http://server/files/token/xxx
	oldFileURL := "file://" + root
	if strings.Contains(msg, oldFileURL) {
		msg = strings.ReplaceAll(msg, oldFileURL, httpPrefix)
		changed = true
	}

	// 2. 替换 JSON 中的裸绝对路径 ("file":"/root/bot/xxx")
	// "file":"/root/bot/xxx" -> "file":"http://server/files/token/xxx"
	oldJsonPath := "\"file\":\"" + root
	newJsonPath := "\"file\":\"" + httpPrefix
	if strings.Contains(msg, oldJsonPath) {
		msg = strings.ReplaceAll(msg, oldJsonPath, newJsonPath)
		changed = true
	}

	// 2b. JSON 中可能有空格: "file": "/root/bot/xxx"
	oldJsonPath2 := "\"file\": \"" + root
	newJsonPath2 := "\"file\": \"" + httpPrefix
	if strings.Contains(msg, oldJsonPath2) {
		msg = strings.ReplaceAll(msg, oldJsonPath2, newJsonPath2)
		changed = true
	}

	// 3. 替换 CQ码中的裸绝对路径 (file=/root/bot/xxx)
	// 不会误匹配已转换的URL，因为规则1的结果是 file=http://... 不含 file=/root/bot
	oldCQPath := "file=" + root
	newCQPath := "file=" + httpPrefix
	if strings.Contains(msg, oldCQPath) {
		msg = strings.ReplaceAll(msg, oldCQPath, newCQPath)
		changed = true
	}

	if changed {
		log.Printf("[FileConv] 转换完成\n")
		if CONFIG.Server.Debug {
			log.Printf("[FileConv] 结果: %s\n", msg)
		}
	}

	return []byte(msg)
}
