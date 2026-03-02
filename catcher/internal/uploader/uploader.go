package uploader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Uploader 数据上传器
type Uploader struct {
	ServerURL   string
	SaveLocally bool
	SaveDir     string
	client      *http.Client
}

// NewUploader 创建上传器
func NewUploader(serverURL string, saveLocally bool, saveDir string) *Uploader {
	return &Uploader{
		ServerURL:   serverURL,
		SaveLocally: saveLocally,
		SaveDir:     saveDir,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Upload 上传数据到服务器
func (u *Uploader) Upload(region, uid, dataType string, data map[string]interface{}) error {
	// 添加时间戳和来源标记
	if _, ok := data["upload_time"]; !ok {
		data["upload_time"] = time.Now().UnixMilli()
	}
	data["source"] = "catcher"
	data["local_source"] = "local_catcher"

	// 保存到本地
	if u.SaveLocally {
		if err := u.saveToLocal(region, uid, dataType, data); err != nil {
			fmt.Printf("[保存] 本地保存失败: %v\n", err)
		}
	}

	// 上传到服务器
	if u.ServerURL == "" {
		return nil
	}

	url := fmt.Sprintf("%s/api/%s/user/%s/upload/%s", u.ServerURL, region, uid, dataType)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回错误 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// saveToLocal 保存数据到本地文件
func (u *Uploader) saveToLocal(region, uid, dataType string, data map[string]interface{}) error {
	dir := filepath.Join(u.SaveDir, region, dataType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, uid+".json")
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, jsonData, 0644)
}

// SaveRawData 保存原始二进制数据
func (u *Uploader) SaveRawData(region, uid, dataType string, rawData []byte) error {
	dir := filepath.Join(u.SaveDir, region, dataType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, uid+".bin")
	fmt.Printf("[保存] 原始数据 -> %s (%d bytes)\n", filePath, len(rawData))
	return os.WriteFile(filePath, rawData, 0644)
}
