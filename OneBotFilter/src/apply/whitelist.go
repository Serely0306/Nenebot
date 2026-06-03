package apply

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type Application struct {
	ID          string `json:"id"`
	GroupID     string `json:"group_id"`
	Applicant   string `json:"applicant"`
	Status      string `json:"status"`
	Visible     *bool  `json:"visible"`
}

type ApplicationFile struct {
	Meta    Meta          `json:"meta"`
	Records []Application `json:"records"`
}

type Meta struct {
	IPBlacklist  []string       `json:"ip_blacklist"`
	IPFakeCounts map[string]int `json:"ip_fake_counts"`
}

type Whitelist struct {
	applicants map[string]bool
	updatedAt  time.Time
	mu         sync.RWMutex
	path       string
}

func NewWhitelist(jsonPath string) *Whitelist {
	w := &Whitelist{
		applicants: make(map[string]bool),
		path:       jsonPath,
	}
	w.refresh()
	return w
}

func (w *Whitelist) Contains(userID string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.applicants[userID]
}

func (w *Whitelist) Refresh() {
	w.refresh()
}

func (w *Whitelist) refresh() {
	data, err := os.ReadFile(w.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[apply] 读取白名单文件失败: %v\n", err)
		}
		return
	}

	var records []Application

	// Try new container format {meta, records} first
	var file ApplicationFile
	if err := json.Unmarshal(data, &file); err == nil && len(file.Records) > 0 {
		records = file.Records
	} else {
		// Fall back to old plain-array format
		var arr []Application
		if err := json.Unmarshal(data, &arr); err == nil {
			records = arr
		} else {
			log.Printf("[apply] 解析白名单文件失败(新旧格式均不匹配)\n")
			return
		}
	}

	applicants := make(map[string]bool)
	for _, r := range records {
		if r.Status == "approved" && r.Applicant != "" && (r.Visible == nil || *r.Visible) {
			applicants[r.Applicant] = true
		}
	}

	w.mu.Lock()
	w.applicants = applicants
	w.updatedAt = time.Now()
	w.mu.Unlock()
}
