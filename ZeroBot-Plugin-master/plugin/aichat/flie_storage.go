package aichat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// FileStorage 文件存储实现
type FileStorage struct {
	dir   string
	cache map[int64]*GroupConfig
	mu    sync.RWMutex
}

// GroupConfig 群组配置
type GroupConfig struct {
	GroupID   int64     `json:"group_id"`
	Rate      uint8     `json:"rate"`        // 触发概率
	Temp      uint8     `json:"temp"`        // 温度 (0-100)
	NoAgent   bool      `json:"no_agent"`    // 是否不使用Agent
	NoRecord  bool      `json:"no_record"`   // 是否不以AI语音输出
	NoReplyAt bool      `json:"no_reply_at"` // 是否不响应@
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	fileStorage *FileStorage
	storageOnce sync.Once
)

// GetFileStorage 获取文件存储实例
func GetFileStorage() *FileStorage {
	storageOnce.Do(func() {
		// 存储目录
		dir := "./data/aichat/storage/"
		if err := os.MkdirAll(dir, 0755); err != nil {
			logrus.Errorf("创建存储目录失败: %v", err)
		}

		fileStorage = &FileStorage{
			dir:   dir,
			cache: make(map[int64]*GroupConfig),
		}

		// 加载已有配置
		fileStorage.loadAll()
		logrus.Infof("文件存储初始化完成，目录: %s", dir)
	})
	return fileStorage
}

// loadAll 加载所有配置
func (fs *FileStorage) loadAll() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	files, err := os.ReadDir(fs.dir)
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// 解析群组ID
		filename := file.Name()
		groupIDStr := filename[:len(filename)-5]
		groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err != nil {
			continue
		}

		// 读取文件
		path := filepath.Join(fs.dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var config GroupConfig
		if err := json.Unmarshal(data, &config); err == nil {
			fs.cache[groupID] = &config
		}
	}
}

// Get 获取群组配置
func (fs *FileStorage) Get(groupID int64) *GroupConfig {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if config, exists := fs.cache[groupID]; exists {
		return config
	}

	// 返回默认配置
	return &GroupConfig{
		GroupID:   groupID,
		Rate:      0,     // 默认触发概率
		Temp:      70,    // 默认温度 70 (0.7)
		NoAgent:   false, // 默认使用Agent
		NoRecord:  false, // 默认使用AI语音输出
		NoReplyAt: false, // 默认响应@
		UpdatedAt: time.Now(),
	}
}

// Save 保存群组配置
func (fs *FileStorage) Save(config *GroupConfig) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	config.UpdatedAt = time.Now()
	fs.cache[config.GroupID] = config

	// 保存到文件
	return fs.saveToFile(config)
}

// saveToFile 保存到文件
func (fs *FileStorage) saveToFile(config *GroupConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%d.json", config.GroupID)
	path := filepath.Join(fs.dir, filename)
	return os.WriteFile(path, data, 0644)
}

// SaveRate 保存触发概率
func (fs *FileStorage) SaveRate(groupID int64, rate uint8) error {
	config := fs.Get(groupID)
	config.Rate = rate
	if config.Rate > 100 {
		config.Rate = 100
	}
	return fs.Save(config)
}

// SaveTemp 保存温度
func (fs *FileStorage) SaveTemp(groupID int64, temp uint8) error {
	config := fs.Get(groupID)
	config.Temp = temp
	if config.Temp > 100 {
		config.Temp = 100
	}
	return fs.Save(config)
}

// SaveNoRecord 保存是否不以AI语音输出
func (fs *FileStorage) SaveNoRecord(groupID int64, noRecord bool) error {
	config := fs.Get(groupID)
	config.NoRecord = noRecord
	return fs.Save(config)
}

// SaveNoAgent 保存是否不使用Agent
func (fs *FileStorage) SaveNoAgent(groupID int64, noAgent bool) error {
	config := fs.Get(groupID)
	config.NoAgent = noAgent
	return fs.Save(config)
}

// SaveNoReplyAt 保存是否不响应@
func (fs *FileStorage) SaveNoReplyAt(groupID int64, noReplyAt bool) error {
	config := fs.Get(groupID)
	config.NoReplyAt = noReplyAt
	return fs.Save(config)
}
