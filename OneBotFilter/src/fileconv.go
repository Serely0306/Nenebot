package onebotfilter

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// filePathPattern 匹配 CQ码和JSON中的 file:// 路径
// 匹配 file:///绝对路径 直到遇到 " , ] 或空白字符
var filePathPattern = regexp.MustCompile(`file://(/[^",\]\s\\]+)`)

// rawPathPattern 匹配 CQ码和JSON中不带 file:// 前缀的裸绝对路径
// 匹配 "file":"/absolute/path" 或 file=/absolute/path 的情况
// 注意：只匹配 file= 或 "file":" 后面跟绝对路径的情况，避免误匹配
var rawPathInJsonPattern = regexp.MustCompile(`"file"\s*:\s*"(/[^"]+)"`)
var rawPathInCQPattern = regexp.MustCompile(`file=(/[^,\]\s]+)`)

// ConvertFileToURL 将消息中的 file:// 路径转换为 HTTP URL
// 仅在文件服务启用时生效
func ConvertFileToURL(msgData []byte) []byte {
	cfg := CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" || cfg.PublicURL == "" {
		return msgData
	}

	// 第一步：处理 file:// 前缀的路径
	msgData = replaceFileURLPaths(msgData, cfg)

	// 第二步：处理 JSON 中不带 file:// 的裸绝对路径 ("file":"/path/...")
	msgData = rawPathInJsonPattern.ReplaceAllFunc(msgData, func(match []byte) []byte {
		submatches := rawPathInJsonPattern.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		filePath := string(submatches[1])
		httpURL := convertPathToURL(filePath, cfg)
		if httpURL == "" {
			return match
		}
		// 重新构建 JSON field: "file":"http://..."
		return []byte(fmt.Sprintf(`"file":"%s"`, httpURL))
	})

	// 第三步：处理 CQ 码中不带 file:// 的裸绝对路径 (file=/path/...)
	msgData = rawPathInCQPattern.ReplaceAllFunc(msgData, func(match []byte) []byte {
		submatches := rawPathInCQPattern.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		filePath := string(submatches[1])
		// 跳过已经是 http/https URL 的情况
		if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
			return match
		}
		httpURL := convertPathToURL(filePath, cfg)
		if httpURL == "" {
			return match
		}
		return []byte("file=" + httpURL)
	})

	return msgData
}

// replaceFileURLPaths 处理 file:// 前缀的路径
func replaceFileURLPaths(msgData []byte, cfg FileServerConfig) []byte {
	return filePathPattern.ReplaceAllFunc(msgData, func(match []byte) []byte {
		submatches := filePathPattern.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		filePath := string(submatches[1])
		httpURL := convertPathToURL(filePath, cfg)
		if httpURL == "" {
			return match
		}
		return []byte(httpURL)
	})
}

// convertPathToURL 将本地文件路径转换为 HTTP URL
// 返回空字符串表示不应转换
func convertPathToURL(filePath string, cfg FileServerConfig) string {
	root := filepath.Clean(cfg.Root)
	publicURL := strings.TrimRight(cfg.PublicURL, "/")
	token := CONFIG.Server.AccessToken

	cleanPath := filepath.Clean(filePath)

	// 安全检查：确保路径在允许的根目录下
	if !strings.HasPrefix(cleanPath, root) {
		if CONFIG.Server.Debug {
			log.Printf("文件路径不在允许的根目录下，跳过转换: %s (root: %s)\n", cleanPath, root)
		}
		return ""
	}

	// 检查文件是否存在
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		if CONFIG.Server.Debug {
			log.Printf("文件不存在，跳过转换: %s\n", cleanPath)
		}
		return ""
	}

	// 计算相对路径
	relPath, err := filepath.Rel(root, cleanPath)
	if err != nil {
		log.Printf("计算相对路径失败: %v\n", err)
		return ""
	}

	// 使用正斜杠确保URL格式正确
	relPath = filepath.ToSlash(relPath)
	// 注意：不要预先URL编码，因为NapCat会自行编码
	// 若预编码则NapCat会二次编码(%→%25)导致404

	// 使用路径内嵌 token 格式: /files/{token}/path
	// 这样 NapCat 不需要额外处理 query parameter
	var httpURL string
	if token != "" {
		httpURL = fmt.Sprintf("%s/files/%s/%s", publicURL, token, relPath)
	} else {
		httpURL = fmt.Sprintf("%s/files/%s", publicURL, relPath)
	}

	if CONFIG.Server.Debug {
		log.Printf("文件路径转换: %s -> %s\n", filePath, httpURL)
	}

	return httpURL
}
