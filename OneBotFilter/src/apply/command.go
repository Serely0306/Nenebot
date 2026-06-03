package apply

import (
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"onebotfilter/src/core"
)

func (m *Module) CanHandle(msg *core.OneBotMessage) bool {
	if !m.cfg.Enabled || msg == nil {
		return false
	}
	text := extractMessageText(msg)
	return strings.HasPrefix(text, "/审核通过") ||
		strings.HasPrefix(text, "/审核拒绝") ||
		strings.HasPrefix(text, "/审核列表") ||
		strings.HasPrefix(text, "/审核查看")
}

func (m *Module) Handle(msg *core.OneBotMessage) map[string]interface{} {
	userID := core.GetMessageUserID(msg)
	if !core.CONFIG.Server.CommandAuth.IsSuperUser(userID) {
		return buildReply(msg, "权限不足，仅超级管理员可操作")
	}

	text := extractMessageText(msg)

	if strings.HasPrefix(text, "/审核列表") {
		return m.handleList(msg)
	}

	if strings.HasPrefix(text, "/审核查看") {
		return m.handleDetail(msg, text)
	}

	parts := strings.Fields(text)
	if len(parts) < 2 {
		return buildReply(msg, "用法：/审核通过 <序号或ID> [备注]  或  /审核拒绝 <序号或ID> [备注]\n查看列表：/审核列表  查看详情：/审核查看 <序号或ID>")
	}

	action := parts[0]
	arg := strings.TrimSpace(parts[1])
	note := ""
	if len(parts) > 2 {
		note = strings.Join(parts[2:], " ")
	}

	file, err := readApplicationFile(m.cfg.ApplicationsPath)
	if err != nil {
		log.Printf("[apply] bot指令读取applications.json失败: %v\n", err)
		return buildReply(msg, "读取申请数据失败")
	}

	pendingRows := filterPendingRows(file.Records)
	appID, seqNum := resolveAppID(arg, pendingRows)

	idx := -1
	for i, rec := range file.Records {
		if rec.ID == appID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return buildReply(msg, fmt.Sprintf("未找到申请 %s", arg))
	}

	rec := &file.Records[idx]
	nowStr := time.Now().UTC().Format(time.RFC3339)

	displayID := appID
	if seqNum > 0 {
		displayID = fmt.Sprintf("#%d (%s)", seqNum, appID)
	}

	switch action {
	case "/审核通过":
		if rec.Status != "pending" {
			return buildReply(msg, fmt.Sprintf("申请 %s 状态为 %s，无法通过", displayID, rec.Status))
		}
		rec.Status = "approved"
		rec.AdminNote = note
		rec.ReviewedAt = &nowStr
		if err := writeApplicationFile(m.cfg.ApplicationsPath, &file); err != nil {
			log.Printf("[apply] 回写失败: %v\n", err)
			return buildReply(msg, "操作失败，请重试")
		}
		log.Printf("[apply] Bot指令: %d 通过了申请 %s\n", userID, appID)
		return buildReply(msg, fmt.Sprintf("已通过申请 %s", displayID))

	case "/审核拒绝":
		if rec.Status != "pending" {
			return buildReply(msg, fmt.Sprintf("申请 %s 状态为 %s，无法拒绝", displayID, rec.Status))
		}
		rec.Status = "rejected"
		rec.AdminNote = note
		rec.ReviewedAt = &nowStr
		if err := writeApplicationFile(m.cfg.ApplicationsPath, &file); err != nil {
			log.Printf("[apply] 回写失败: %v\n", err)
			return buildReply(msg, "操作失败，请重试")
		}
		log.Printf("[apply] Bot指令: %d 拒绝了申请 %s, 备注: %s\n", userID, appID, note)
		return buildReply(msg, fmt.Sprintf("已拒绝申请 %s", displayID))
	}

	return buildReply(msg, "未知操作")
}

func (m *Module) handleList(msg *core.OneBotMessage) map[string]interface{} {
	file, err := readApplicationFile(m.cfg.ApplicationsPath)
	if err != nil {
		log.Printf("[apply] 读取applications.json失败: %v\n", err)
		return buildReply(msg, "读取申请数据失败")
	}

	pendingRows := filterPendingRows(file.Records)
	var rows []ReviewRow
	for i, r := range pendingRows {
		rows = append(rows, ReviewRow{
			Index:       i + 1,
			ID:          r.ID,
			GroupID:     r.GroupID,
			GroupName:   r.GroupName,
			MemberCount: r.MemberCount,
			Applicant:   r.Applicant,
			Nickname:    r.ApplicantNickname,
			Purpose:     r.Purpose,
			CreatedAt:   r.CreatedAt,
		})
	}

	imageBytes, err := RenderReviewList(m.fontPath, RenderReviewInput{
		Title: fmt.Sprintf("待审核列表 (%d)", len(rows)),
		Rows:  rows,
	})
	if err != nil {
		log.Printf("[apply] 渲染审核列表图片失败: %v\n", err)
		return buildReply(msg, fmt.Sprintf("图片生成失败: %v", err))
	}

	return buildImageReply(msg, imageBytes)
}

func (m *Module) handleDetail(msg *core.OneBotMessage, text string) map[string]interface{} {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return buildReply(msg, "用法：/审核查看 <序号或ID>")
	}
	arg := strings.TrimSpace(parts[1])

	file, err := readApplicationFile(m.cfg.ApplicationsPath)
	if err != nil {
		log.Printf("[apply] 读取applications.json失败: %v\n", err)
		return buildReply(msg, "读取申请数据失败")
	}

	pendingRows := filterPendingRows(file.Records)
	appID, seqNum := resolveAppID(arg, pendingRows)

	idx := -1
	for i, rec := range file.Records {
		if rec.ID == appID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return buildReply(msg, fmt.Sprintf("未找到申请 %s", arg))
	}

	rec := file.Records[idx]

	statusLabel := "待审核"
	switch rec.Status {
	case "approved":
		statusLabel = "已通过"
	case "rejected":
		statusLabel = "已拒绝"
	}

	verifyText := "未验证"
	if rec.Verified != nil {
		if *rec.Verified {
			verifyText = "✅ 群存在"
		} else if rec.Status == "rejected" {
			verifyText = "❌ 群号不存在"
		} else {
			verifyText = "⚠️ 无法验证"
		}
	}

	groupDisplay := rec.GroupID
	if rec.GroupName != "" {
		groupDisplay = fmt.Sprintf("%s (%s)", rec.GroupName, rec.GroupID)
	}

	applicantDisplay := rec.Applicant
	if rec.ApplicantNickname != "" {
		applicantDisplay = fmt.Sprintf("%s (%s)", rec.ApplicantNickname, rec.Applicant)
	}

	createdStr := rec.CreatedAt
	if t, err := time.Parse(time.RFC3339, rec.CreatedAt); err == nil {
		createdStr = t.In(time.Local).Format("2006-01-02 15:04:05")
	}

	msgBody := fmt.Sprintf(
		"[Bot加群审核] 申请详情\n\n申请ID：%s\n状态：%s\n申请人：%s\n目标群：%s  验证状态：%s\n群人数：%d\n拉群目的：%s\n提交时间：%s",
		rec.ID, statusLabel, applicantDisplay, groupDisplay, verifyText, rec.MemberCount, rec.Purpose, createdStr,
	)

	if rec.AdminNote != "" {
		msgBody += fmt.Sprintf("\n管理员备注：%s", rec.AdminNote)
	}
	if rec.VerificationNote != nil && *rec.VerificationNote != "" {
		msgBody += fmt.Sprintf("\n验证详情：%s", *rec.VerificationNote)
	}
	if rec.ReviewedAt != nil {
		reviewedStr := *rec.ReviewedAt
		if t, err := time.Parse(time.RFC3339, *rec.ReviewedAt); err == nil {
			reviewedStr = t.In(time.Local).Format("2006-01-02 15:04:05")
		}
		msgBody += fmt.Sprintf("\n处理时间：%s", reviewedStr)
	}

	if rec.Status == "pending" {
		actionID := appID
		if seqNum > 0 {
			actionID = strconv.Itoa(seqNum)
		}
		msgBody += fmt.Sprintf("\n\n处理指令：/审核通过 %s  或  /审核拒绝 %s", actionID, actionID)
	}

	return buildReply(msg, msgBody)
}

func filterPendingRows(records []appRecord) []appRecord {
	var result []appRecord
	for _, r := range records {
		if r.Status == "pending" && (r.Visible == nil || *r.Visible) {
			result = append(result, r)
		}
	}
	return result
}

func resolveAppID(input string, pendingRows []appRecord) (string, int) {
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(pendingRows) {
		return pendingRows[n-1].ID, n
	}
	return input, 0
}

func extractMessageText(msg *core.OneBotMessage) string {
	switch msg.Partial.MessageFormat {
	case "array":
		for _, seg := range msg.Partial.MessageArray {
			if seg.Type == "text" {
				if t, ok := seg.Data["text"].(string); ok {
					return strings.TrimSpace(t)
				}
			}
		}
	case "string":
		return strings.TrimSpace(msg.Partial.MessageString)
	}
	return ""
}

func buildReply(msg *core.OneBotMessage, text string) map[string]interface{} {
	if msg.Partial.MessageType == "private" {
		return map[string]interface{}{
			"action": "send_private_msg",
			"params": map[string]interface{}{
				"user_id": msg.Partial.UserID,
				"message": text,
			},
		}
	}
	return map[string]interface{}{
		"action": "send_group_msg",
		"params": map[string]interface{}{
			"group_id": msg.Partial.GroupID,
			"message":  text,
		},
	}
}

func buildImageReply(msg *core.OneBotMessage, imageBytes []byte) map[string]interface{} {
	imageValue := "base64://" + base64.StdEncoding.EncodeToString(imageBytes)
	params := map[string]interface{}{
		"message": []map[string]interface{}{
			{
				"type": "image",
				"data": map[string]interface{}{
					"file": imageValue,
				},
			},
		},
	}
	if msg.Partial.MessageType == "private" {
		params["user_id"] = msg.Partial.UserID
		return map[string]interface{}{"action": "send_private_msg", "params": params}
	}
	params["group_id"] = msg.Partial.GroupID
	return map[string]interface{}{"action": "send_group_msg", "params": params}
}
