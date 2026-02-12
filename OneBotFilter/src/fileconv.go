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

// ConvertFileToURL 将消息中的 file:// 路径转换为 HTTP URL
// 仅在文件服务启用时生效
func ConvertFileToURL(msgData []byte) []byte {
	cfg := CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" || cfg.PublicURL == "" {
		return msgData
	}

	root := filepath.Clean(cfg.Root)
	publicURL := strings.TrimRight(cfg.PublicURL, "/")
	token := CONFIG.Server.AccessToken

	result := filePathPattern.ReplaceAllFunc(msgData, func(match []byte) []byte {
		// 提取完整的文件路径
		submatches := filePathPattern.FindSubmatch(match)
		if len(submatches) < 2 {
			return match
		}

		filePath := string(submatches[1])
		cleanPath := filepath.Clean(filePath)

		// 安全检查：确保路径在允许的根目录下
		if !strings.HasPrefix(cleanPath, root) {
			if CONFIG.Server.Debug {
				log.Printf("文件路径不在允许的根目录下，跳过转换: %s (root: %s)\n", cleanPath, root)
			}
			return match
		}

		// 检查文件是否存在
		if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
			if CONFIG.Server.Debug {
				log.Printf("文件不存在，跳过转换: %s\n", cleanPath)
			}
			return match
		}

		// 计算相对路径
		relPath, err := filepath.Rel(root, cleanPath)
		if err != nil {
			log.Printf("计算相对路径失败: %v\n", err)
			return match
		}

		// 构建HTTP URL
		// 使用正斜杠确保URL格式正确
		relPath = filepath.ToSlash(relPath)
		var url string
		if token != "" {
			url = fmt.Sprintf("%s/files/%s?access_token=%s", publicURL, relPath, token)
		} else {
			url = fmt.Sprintf("%s/files/%s", publicURL, relPath)
		}

		if CONFIG.Server.Debug {
			log.Printf("文件路径转换: file://%s -> %s\n", filePath, url)
		}

		return []byte(url)
	})

	return result
}
