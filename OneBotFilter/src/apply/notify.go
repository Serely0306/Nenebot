package apply

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"onebotfilter/src/core"
)

type notifyRequest struct {
	AppID string `json:"app_id"`
}

type applicationFile struct {
	Meta    metaData    `json:"meta"`
	Records []appRecord `json:"records"`
}

type metaData struct {
	IPBlacklist  []string       `json:"ip_blacklist"`
	IPFakeCounts map[string]int `json:"ip_fake_counts"`
}

type appRecord struct {
	ID                string  `json:"id"`
	GroupID           string  `json:"group_id"`
	GroupName         string  `json:"group_name"`
	MemberCount       int     `json:"member_count"`
	Purpose           string  `json:"purpose"`
	Applicant         string  `json:"applicant"`
	ApplicantNickname string  `json:"applicant_nickname"`
	ClientIP          string  `json:"client_ip"`
	Verified          *bool   `json:"verified"`
	VerifiedAt        *string `json:"verified_at"`
	VerificationNote  *string `json:"verification_note"`
	Status            string  `json:"status"`
	AdminNote         string  `json:"admin_note"`
	Visible           *bool   `json:"visible"`
	CreatedAt         string  `json:"created_at"`
	ReviewedAt        *string `json:"reviewed_at"`
}

var notifyMu sync.Mutex

func fsLockPath(appPath string) string {
	return filepath.Join(filepath.Dir(appPath), ".applications.lock")
}

func acquireFsLock(appPath string, timeout time.Duration) (*os.File, error) {
	lockPath := fsLockPath(appPath)
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("acquire fs lock timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func releaseFsLock(f *os.File, appPath string) {
	f.Close()
	os.Remove(fsLockPath(appPath))
}

func (m *Module) HandleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, _ := io.ReadAll(r.Body)
	r.Body.Close()

	var req notifyRequest
	if err := json.Unmarshal(body, &req); err != nil || req.AppID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	log.Printf("[apply] 收到 notify 请求: app_id=%s\n", req.AppID)

	// 1. Read applications.json
	file, err := readApplicationFile(m.cfg.ApplicationsPath)
	if err != nil {
		log.Printf("[apply] 读取 applications.json 失败: %v\n", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// 2. Find the application record
	idx := -1
	for i, rec := range file.Records {
		if rec.ID == req.AppID {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}

	rec := &file.Records[idx]
	if rec.Status != "pending" || rec.Verified != nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"already_verified"}`))
		return
	}

	// 3. Verify group (no lock held — network call may take seconds)
	verifyResult, verifyNote, memberCount, groupName := verifyGroup(m.wss, rec.GroupID)

	// 4. Lock, re-read, apply, write (prevents lost updates against Python process)
	fsLock, lockErr := acquireFsLock(m.cfg.ApplicationsPath, 5*time.Second)
	if lockErr != nil {
		log.Printf("[apply] 获取文件锁失败: %v\n", lockErr)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer releaseFsLock(fsLock, m.cfg.ApplicationsPath)

	notifyMu.Lock()
	defer notifyMu.Unlock()

	file2, err := readApplicationFile(m.cfg.ApplicationsPath)
	if err != nil {
		log.Printf("[apply] 重新读取 applications.json 失败: %v\n", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	idx2 := -1
	for i, r := range file2.Records {
		if r.ID == req.AppID {
			idx2 = i
			break
		}
	}
	if idx2 < 0 {
		http.Error(w, "application not found", http.StatusNotFound)
		return
	}

	rec2 := &file2.Records[idx2]
	if rec2.Status != "pending" || rec2.Verified != nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"already_verified"}`))
		return
	}

	if verifyResult {
		if memberCount > 0 {
			rec2.MemberCount = memberCount
		}
		if groupName != "" {
			rec2.GroupName = groupName
		}
		if applicantID, err := strconv.ParseInt(rec2.Applicant, 10, 64); err == nil {
			if resp, err := m.wss.CallAPI("get_stranger_info", map[string]interface{}{
				"user_id":  applicantID,
				"no_cache": true,
			}, 3*time.Second); err == nil {
				if data, ok := resp["data"].(map[string]interface{}); ok {
					if nick, ok := data["nickname"].(string); ok {
						rec2.ApplicantNickname = nick
					}
				}
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec2.Verified = &verifyResult
	rec2.VerifiedAt = &now
	rec2.VerificationNote = &verifyNote

	if !verifyResult && strings.Contains(verifyNote, "不存在") {
		ip := rec2.ClientIP
		if file2.Meta.IPFakeCounts == nil {
			file2.Meta.IPFakeCounts = make(map[string]int)
		}
		file2.Meta.IPFakeCounts[ip] = file2.Meta.IPFakeCounts[ip] + 1
		count := file2.Meta.IPFakeCounts[ip]
		log.Printf("[apply] IP %s 虚假申请计数: %d\n", ip, count)
		if count >= 3 {
			inList := false
			for _, b := range file2.Meta.IPBlacklist {
				if b == ip {
					inList = true
					break
				}
			}
			if !inList {
				file2.Meta.IPBlacklist = append(file2.Meta.IPBlacklist, ip)
				log.Printf("[apply] IP %s 已加入黑名单\n", ip)
			}
		}
		rec2.Status = "rejected"
		rec2.AdminNote = "群号不存在，虚假信息计次+1，超过三次封锁对应IP，请谨慎提交"
	} else if !verifyResult {
		rec2.AdminNote = "无法验证群号存在（可能原因：群设置了不允许被搜索、群已解散、网络超时），待人工核实中"
	}

	if err := writeApplicationFile(m.cfg.ApplicationsPath, &file2); err != nil {
		log.Printf("[apply] 回写 applications.json 失败: %v\n", err)
	}

	go m.notifySuperUsers(rec2, verifyResult, verifyNote)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func verifyGroup(wss *core.WsServer, groupID string) (bool, string, int, string) {
	groupIDInt, err := strconv.ParseInt(groupID, 10, 64)
	if err != nil {
		return false, fmt.Sprintf("群号格式无效: %s", groupID), 0, ""
	}

	resp, err := wss.CallAPI("get_group_info", map[string]interface{}{
		"group_id": groupIDInt,
		"no_cache": true,
	}, 5*time.Second)

	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "not found") ||
			strings.Contains(errStr, "不存在") ||
			strings.Contains(errStr, "invalid") {
			return false, fmt.Sprintf("群号 %s 不存在", groupID), 0, ""
		}
		return false, fmt.Sprintf("验证超时或网络错误: %v", err), 0, ""
	}

	// Check API-level failure (OneBot returns status="failed" with retcode != 0)
	if status, _ := resp["status"].(string); status == "failed" {
		return false, fmt.Sprintf("群号 %s 不存在", groupID), 0, ""
	}

	if retcode, ok := resp["retcode"].(float64); ok && retcode != 0 {
		return false, fmt.Sprintf("群号 %s 不存在", groupID), 0, ""
	}

	data, _ := resp["data"].(map[string]interface{})
	if data == nil || data["group_id"] == nil {
		return false, fmt.Sprintf("群号 %s 无法获取信息（可能被隐私设置隐藏）", groupID), 0, ""
	}

	mc := 0
	if v, ok := data["member_count"].(float64); ok {
		mc = int(v)
	}
	gn := ""
	if v, ok := data["group_name"].(string); ok {
		gn = v
	}
	return true, "", mc, gn
}

func (m *Module) notifySuperUsers(rec *appRecord, verified bool, verifyNote string) {
	superUsers := core.CONFIG.Server.CommandAuth.SuperUsers
	if len(superUsers) == 0 {
		return
	}

	verifyText := "⚠️ 无法验证"
	if verified {
		verifyText = "✅ 群存在"
	} else if rec.Status == "rejected" {
		verifyText = "❌ 群号不存在（已自动拒绝）"
	}

	reviewURL := m.cfg.ReviewURL
	if reviewURL == "" {
		reviewURL = "(未配置 review_url)"
	}

	groupDisplay := rec.GroupID
	if rec.GroupName != "" {
		groupDisplay = fmt.Sprintf("%s (%s)", rec.GroupName, rec.GroupID)
	}
	msg := fmt.Sprintf(
		"[Bot加群审核] 新申请待处理\n\n申请ID：%s\n申请人QQ：%s\n目标群：%s  验证状态：%s\n群人数：%d\n拉群目的：%s\n\n处理指令：/审核通过 %s  或  /审核拒绝 %s\n或前往审核：%s",
		rec.ID, rec.Applicant, groupDisplay, verifyText, rec.MemberCount, rec.Purpose,
		rec.ID, rec.ID, reviewURL,
	)

	for _, su := range superUsers {
		_, err := m.wss.CallAPI("send_private_msg", map[string]interface{}{
			"user_id": su,
			"message": msg,
		}, 5*time.Second)
		if err != nil {
			log.Printf("[apply] 通知 SuperUser %d 失败: %v\n", su, err)
		} else {
			log.Printf("[apply] 已通知 SuperUser %d: app_id=%s\n", su, rec.ID)
		}
	}
}

func readApplicationFile(path string) (applicationFile, error) {
	var file applicationFile
	data, err := os.ReadFile(path)
	if err != nil {
		return file, err
	}
	// Try new container format first
	if err := json.Unmarshal(data, &file); err != nil {
		// Fall back to old plain array
		var records []appRecord
		if err2 := json.Unmarshal(data, &records); err2 != nil {
			return file, fmt.Errorf("无法解析 applications.json (新旧格式均失败)")
		}
		file.Records = records
		file.Meta = metaData{}
	}
	return file, nil
}

func writeApplicationFile(path string, file *applicationFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	success := false
	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmpFile.Write(data); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}
