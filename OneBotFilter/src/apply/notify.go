package apply

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

	// 3. Verify group
	verifyResult, verifyNote, memberCount, groupName := verifyGroup(m.wss, rec.GroupID)
	if verifyResult {
		if memberCount > 0 {
			rec.MemberCount = memberCount
		}
		if groupName != "" {
			rec.GroupName = groupName
		}
		// Fetch applicant nickname for display
		if applicantID, err := strconv.ParseInt(rec.Applicant, 10, 64); err == nil {
			if resp, err := m.wss.CallAPI("get_stranger_info", map[string]interface{}{
				"user_id":  applicantID,
				"no_cache": true,
			}, 3*time.Second); err == nil {
				if data, ok := resp["data"].(map[string]interface{}); ok {
					if nick, ok := data["nickname"].(string); ok {
						rec.ApplicantNickname = nick
					}
				}
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec.Verified = &verifyResult
	rec.VerifiedAt = &now
	rec.VerificationNote = &verifyNote

	// 4. Fraud tracking
	if !verifyResult && strings.Contains(verifyNote, "不存在") {
		// Confirmed fake
		ip := rec.ClientIP
		if file.Meta.IPFakeCounts == nil {
			file.Meta.IPFakeCounts = make(map[string]int)
		}
		file.Meta.IPFakeCounts[ip] = file.Meta.IPFakeCounts[ip] + 1
		count := file.Meta.IPFakeCounts[ip]
		log.Printf("[apply] IP %s 虚假申请计数: %d\n", ip, count)
		if count >= 3 {
			inList := false
			for _, b := range file.Meta.IPBlacklist {
				if b == ip {
					inList = true
					break
				}
			}
			if !inList {
				file.Meta.IPBlacklist = append(file.Meta.IPBlacklist, ip)
				log.Printf("[apply] IP %s 已加入黑名单\n", ip)
			}
		}
		rec.Status = "rejected"
		rec.AdminNote = "群号不存在，虚假信息计次+1，超过三次封锁对应IP，请谨慎提交"
	} else if !verifyResult {
		// Uncertain
		rec.AdminNote = "无法验证群号存在（可能原因：群设置了不允许被搜索、群已解散、网络超时），待人工核实中"
	}

	// 5. Write back
	if err := writeApplicationFile(m.cfg.ApplicationsPath, &file); err != nil {
		log.Printf("[apply] 回写 applications.json 失败: %v\n", err)
	}

	// 6. Notify SuperUsers (async, don't block response)
	go m.notifySuperUsers(rec, verifyResult, verifyNote)

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
	notifyMu.Lock()
	defer notifyMu.Unlock()
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
