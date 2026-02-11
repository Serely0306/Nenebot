package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
)

// readResponseBody 读取响应体（处理压缩）
func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	// 读取原始 body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// 恢复 body 以便后续使用
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// 如果是 gzip 压缩，解压
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, nil // 可能不是真正的 gzip，返回原始数据
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return body, nil
		}
		return decompressed, nil
	}

	return body, nil
}
