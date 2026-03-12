package onebotfilter

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ConvertFileToBase64 在保留原有 HTTP 转换逻辑的前提下，提供可选的 base64 文件转译。
func ConvertFileToBase64(msgData []byte) []byte {
	cfg := CONFIG.Server.FileServer
	if !cfg.Base64Enabled || cfg.Root == "" {
		return msgData
	}

	root := strings.TrimRight(cfg.Root, "/")
	msg := string(msgData)
	cache := make(map[string]string)
	changed := false

	for strings.Contains(msg, "file:////") {
		msg = strings.ReplaceAll(msg, "file:////", "file:///")
		changed = true
	}

	var replaced bool
	msg, replaced = replaceQuotedFileValue(msg, `"file":"`, root, cache, cfg.Base64MaxSize)
	changed = changed || replaced

	msg, replaced = replaceQuotedFileValue(msg, `"file": "`, root, cache, cfg.Base64MaxSize)
	changed = changed || replaced

	msg, replaced = replaceFileURIValue(msg, root, cache, cfg.Base64MaxSize)
	changed = changed || replaced

	msg, replaced = replaceCQFileValue(msg, root, cache, cfg.Base64MaxSize)
	changed = changed || replaced

	if changed {
		log.Printf("[FileConv] base64 转换完成\n")
		if CONFIG.Server.Debug {
			log.Printf("[FileConv] base64 结果: %s\n", msg)
		}
	}

	return []byte(msg)
}

// ConvertFileToURL 将消息中的 file:// 路径转换为 HTTP URL。
// 保留现有的字符串替换逻辑，不引入正则，避免 UTF-8 路径上的额外问题。
func ConvertFileToURL(msgData []byte) []byte {
	cfg := CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" || cfg.PublicURL == "" {
		return msgData
	}

	root := strings.TrimRight(cfg.Root, "/")
	publicURL := strings.TrimRight(cfg.PublicURL, "/")
	token := CONFIG.Server.AccessToken

	var httpPrefix string
	if token != "" {
		httpPrefix = fmt.Sprintf("%s/files/%s", publicURL, token)
	} else {
		httpPrefix = fmt.Sprintf("%s/files", publicURL)
	}

	msg := string(msgData)
	changed := false

	// 先归一化 file:////+ 为 file:///，避免绝对路径导致多余斜杠。
	for strings.Contains(msg, "file:////") {
		msg = strings.ReplaceAll(msg, "file:////", "file:///")
		changed = true
	}

	// 替换 file:// 前缀路径。
	oldFileURL := "file://" + root
	if strings.Contains(msg, oldFileURL) {
		msg = strings.ReplaceAll(msg, oldFileURL, httpPrefix)
		changed = true
	}

	// 替换 JSON 中的绝对路径："file":"/root/bot/xxx"。
	oldJSONPath := `"file":"` + root
	newJSONPath := `"file":"` + httpPrefix
	if strings.Contains(msg, oldJSONPath) {
		msg = strings.ReplaceAll(msg, oldJSONPath, newJSONPath)
		changed = true
	}

	// JSON 中可能带空格："file": "/root/bot/xxx"。
	oldJSONPathWithSpace := `"file": "` + root
	newJSONPathWithSpace := `"file": "` + httpPrefix
	if strings.Contains(msg, oldJSONPathWithSpace) {
		msg = strings.ReplaceAll(msg, oldJSONPathWithSpace, newJSONPathWithSpace)
		changed = true
	}

	// 替换 CQ 码中的绝对路径：file=/root/bot/xxx。
	oldCQPath := "file=" + root
	newCQPath := "file=" + httpPrefix
	if strings.Contains(msg, oldCQPath) {
		msg = strings.ReplaceAll(msg, oldCQPath, newCQPath)
		changed = true
	}

	if changed {
		log.Printf("[FileConv] HTTP 转换完成\n")
		if CONFIG.Server.Debug {
			log.Printf("[FileConv] HTTP 结果: %s\n", msg)
		}
	}

	return []byte(msg)
}

func replaceQuotedFileValue(msg string, marker string, root string, cache map[string]string, maxSize int64) (string, bool) {
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		valueStart := start + len(marker)
		valueEndOffset := strings.Index(msg[valueStart:], `"`)
		if valueEndOffset < 0 {
			break
		}

		valueEnd := valueStart + valueEndOffset
		converted, ok := encodeFileReference(msg[valueStart:valueEnd], root, cache, maxSize)
		if !ok {
			searchIndex = valueEnd
			continue
		}

		msg = msg[:valueStart] + converted + msg[valueEnd:]
		searchIndex = valueStart + len(converted)
		changed = true
	}

	return msg, changed
}

func replaceFileURIValue(msg string, root string, cache map[string]string, maxSize int64) (string, bool) {
	const marker = "file://"
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		end := findValueEnd(msg, start, "\"' \r\n\t,]}")
		converted, ok := encodeFileReference(msg[start:end], root, cache, maxSize)
		if !ok {
			searchIndex = end
			continue
		}

		msg = msg[:start] + converted + msg[end:]
		searchIndex = start + len(converted)
		changed = true
	}

	return msg, changed
}

func replaceCQFileValue(msg string, root string, cache map[string]string, maxSize int64) (string, bool) {
	const marker = "file="
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		valueStart := start + len(marker)
		end := findValueEnd(msg, valueStart, ",] \r\n\t")
		converted, ok := encodeFileReference(msg[valueStart:end], root, cache, maxSize)
		if !ok {
			searchIndex = end
			continue
		}

		msg = msg[:valueStart] + converted + msg[end:]
		searchIndex = valueStart + len(converted)
		changed = true
	}

	return msg, changed
}

func findValueEnd(msg string, start int, terminators string) int {
	for i := start; i < len(msg); i++ {
		if strings.ContainsRune(terminators, rune(msg[i])) {
			return i
		}
	}
	return len(msg)
}

func encodeFileReference(value string, root string, cache map[string]string, maxSize int64) (string, bool) {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "base64://") {
		return "", false
	}

	pathValue := value
	pathValue = strings.TrimPrefix(pathValue, "file://")
	if pathValue == "" {
		return "", false
	}

	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(pathValue)
	if !isSubPath(cleanRoot, cleanPath) {
		return "", false
	}

	if cached, ok := cache[cleanPath]; ok {
		return cached, true
	}

	if maxSize > 0 {
		fileInfo, err := os.Stat(cleanPath)
		if err != nil {
			log.Printf("[FileConv] 获取文件大小失败: %s, err=%v\n", cleanPath, err)
			return "", false
		}
		if fileInfo.Size() > maxSize {
			log.Printf("[FileConv] 文件超过 base64 大小限制，跳过转换: %s, size=%d, limit=%d\n", cleanPath, fileInfo.Size(), maxSize)
			return "", false
		}
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		log.Printf("[FileConv] 读取文件失败: %s, err=%v\n", cleanPath, err)
		return "", false
	}

	converted := "base64://" + base64.StdEncoding.EncodeToString(content)
	cache[cleanPath] = converted
	return converted, true
}

func isSubPath(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "..")
}
