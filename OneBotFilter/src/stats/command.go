package stats

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"onebotfilter/src/core"
)

func ParseStatsArgs(args []string, now time.Time) (bool, DateRange, error) {
	if len(args) == 0 {
		r, err := ParseTimeExpr("", now)
		return false, r, err
	}

	if isGlobalStatsKeyword(args[0]) {
		if len(args) == 1 {
			r, err := ParseTimeExpr("", now)
			return true, r, err
		}
		if len(args) == 2 {
			r, err := ParseTimeExpr(args[1], now)
			return true, r, err
		}
		return false, DateRange{}, fmt.Errorf("用法：/stats [时间] 或 /stats global [时间]")
	}
	if len(args) != 1 {
		return false, DateRange{}, fmt.Errorf("用法：/stats [时间] 或 /stats global [时间]")
	}
	if !isTimeExpr(args[0]) {
		return false, DateRange{}, fmt.Errorf("用法：/stats [时间] 或 /stats global [时间]")
	}
	r, err := ParseTimeExpr(args[0], now)
	return false, r, err
}

func isGlobalStatsKeyword(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "global" || text == "全局"
}

func ParseTimeExpr(expr string, now time.Time) (DateRange, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case expr == "":
		day := midnight(now)
		return buildDayRange(day, "今日"), nil
	case expr == "all":
		return DateRange{Mode: ModeAll, Label: "总计"}, nil
	case strings.Contains(expr, "~"):
		parts := strings.SplitN(expr, "~", 2)
		start, end, err := parseRangeEndpoints(parts[0], parts[1], now)
		if err != nil {
			return DateRange{}, err
		}
		return buildWindowRange(start, end), nil
	default:
		day, label, err := parseSingleDay(expr, now)
		if err != nil {
			return DateRange{}, err
		}
		return buildDayRange(day, label), nil
	}
}

func isTimeExpr(expr string) bool {
	_, err := ParseTimeExpr(expr, time.Now())
	return err == nil
}

func parseRangeEndpoints(left, right string, now time.Time) (time.Time, time.Time, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("区间时间不能为空")
	}

	if isDateLiteral(left) != isDateLiteral(right) {
		return time.Time{}, time.Time{}, fmt.Errorf("区间时间格式必须一致")
	}

	start, _, err := parseSingleDay(left, now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, _, err := parseSingleDay(right, now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if start.After(end) {
		start, end = end, start
	}
	return start, end, nil
}

func parseSingleDay(expr string, now time.Time) (time.Time, string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		day := midnight(now)
		return day, "今日", nil
	}

	if strings.HasPrefix(expr, "-") {
		offset, err := strconv.Atoi(expr)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("无效偏移时间: %s", expr)
		}
		day := midnight(now).AddDate(0, 0, offset)
		switch offset {
		case -1:
			return day, "昨日", nil
		default:
			return day, day.Format("2006-01-02"), nil
		}
	}

	day, err := time.ParseInLocation("2006-01-02", expr, now.Location())
	if err != nil {
		return time.Time{}, "", fmt.Errorf("无效日期: %s", expr)
	}
	return midnight(day), day.Format("2006-01-02"), nil
}

func buildDayRange(day time.Time, label string) DateRange {
	return DateRange{
		Mode:      ModeDay,
		Start:     midnight(day),
		End:       midnight(day),
		Label:     label,
		StartDate: midnight(day).Format("2006-01-02"),
		EndDate:   midnight(day).Format("2006-01-02"),
	}
}

func buildWindowRange(start, end time.Time) DateRange {
	start = midnight(start)
	end = midnight(end)
	return DateRange{
		Mode:      ModeDay,
		Start:     start,
		End:       end,
		Label:     fmt.Sprintf("%s~%s", start.Format("2006-01-02"), end.Format("2006-01-02")),
		StartDate: start.Format("2006-01-02"),
		EndDate:   end.Format("2006-01-02"),
	}
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func isDateLiteral(expr string) bool {
	if expr == "" || strings.HasPrefix(expr, "-") {
		return false
	}
	_, err := time.Parse("2006-01-02", expr)
	return err == nil
}

func (m *Module) TryHandle(msg *core.OneBotMessage) (bool, map[string]interface{}) {
	text := extractCommandText(msg)
	switch {
	case text == "/总发言榜":
		if msg.Partial.MessageType != "group" {
			return true, buildReply(msg, "发言榜仅支持群聊使用")
		}
		return true, m.handleRank(msg, DateRange{Mode: ModeAll, Label: "总计"})
	case text == "/发言榜" || text == "/今日发言榜" || strings.HasPrefix(text, "/发言榜 "):
		if msg.Partial.MessageType != "group" {
			return true, buildReply(msg, "发言榜仅支持群聊使用")
		}
		expr := strings.TrimSpace(strings.TrimPrefix(text, "/发言榜"))
		if text == "/今日发言榜" {
			expr = ""
		}
		r, err := ParseTimeExpr(expr, time.Now())
		if err != nil {
			return true, buildReply(msg, fmt.Sprintf("时间参数错误: %v", err))
		}
		return true, m.handleRank(msg, r)
	case text == "/stats" || strings.HasPrefix(text, "/stats "):
		if !m.isSuperUser(msg.Partial.UserID) && !m.isSuperUser(msg.Partial.Sender.UserID) {
			return true, nil
		}
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(text, "/stats")))
		globalView, r, err := ParseStatsArgs(args, time.Now())
		if err != nil {
			return true, buildReply(msg, err.Error())
		}
		if globalView {
			return true, m.handleGlobalStats(msg, r)
		}
		return true, m.handleStats(msg, r)
	default:
		return false, nil
	}
}

func (m *Module) CanHandle(msg *core.OneBotMessage) bool {
	text := extractCommandText(msg)
	switch {
	case text == "/总发言榜":
		return true
	case text == "/发言榜" || text == "/今日发言榜" || strings.HasPrefix(text, "/发言榜 "):
		return true
	case text == "/stats" || strings.HasPrefix(text, "/stats "):
		return true
	default:
		return false
	}
}

func (m *Module) Handle(msg *core.OneBotMessage) map[string]interface{} {
	_, response := m.TryHandle(msg)
	return response
}

func (m *Module) handleRank(msg *core.OneBotMessage, r DateRange) map[string]interface{} {
	sessionType, sessionID := sessionFromMessage(msg)
	summary, err := m.store.QuerySessionSummary(sessionType, sessionID, r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("统计查询失败: %v", err))
	}
	if summary.RecvCount == 0 {
		return buildReply(msg, "该时间范围暂无消息记录")
	}

	rows, err := m.store.QueryUserRank(sessionType, sessionID, r, m.rankLimit())
	if err != nil {
		return buildReply(msg, fmt.Sprintf("发言榜查询失败: %v", err))
	}

	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	avatars := fetchUserAvatars(userIDs)

	rankRows := make([]RankRow, 0, len(rows))
	for i, row := range rows {
		name := m.displayName(msg.Partial.GroupID, row.UserID, row.Snapshot, sessionType == "group")
		rankRows = append(rankRows, RankRow{
			Index:   i + 1,
			UserID:  row.UserID,
			Name:    name,
			Count:   row.RecvCount,
			Percent: percentage(row.RecvCount, summary.RecvCount),
			Avatar:  avatars[row.UserID],
		})
	}

	imageBytes, err := RenderRankImage(m.fontPath, RenderRankInput{
		Title:       rankTitle(r),
		SessionName: m.messageSessionDisplayName(msg),
		RangeLabel:  rangeLabel(r),
		TotalCount:  summary.RecvCount,
		Rows:        rankRows,
	})
	if err != nil {
		return buildReply(msg, fmt.Sprintf("发言榜图片生成失败: %v", err))
	}
	return buildImageReply(msg, imageBytes)
}

func (m *Module) handleStats(msg *core.OneBotMessage, r DateRange) map[string]interface{} {
	sessionType, sessionID := sessionFromMessage(msg)
	summary, err := m.store.QuerySessionSummary(sessionType, sessionID, r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("统计查询失败: %v", err))
	}

	botRows, err := m.store.QueryBotSend(sessionType, sessionID, r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("bot 统计查询失败: %v", err))
	}

	if summary.RecvCount == 0 && len(botRows) == 0 && summary.SendCount == 0 {
		return buildReply(msg, "该时间范围暂无统计记录")
	}

	renderRows := make([]RankRow, 0, len(botRows))
	for i, row := range botRows {
		renderRows = append(renderRows, RankRow{
			Index:   i + 1,
			Name:    row.BotName,
			Count:   row.SendCount,
			Percent: percentage(row.SendCount, max64(1, summary.BotSendCount)),
		})
	}

	imageBytes, err := RenderStatsImage(m.fontPath, RenderStatsInput{
		Title:             statsTitle(r),
		SessionName:       m.messageSessionDisplayName(msg),
		RangeLabel:        rangeLabel(r),
		SessionAvatar:     sessionAvatar(sessionType, sessionID),
		RecvCount:         summary.RecvCount,
		SendCount:         summary.SendCount,
		BotSendCount:      summary.BotSendCount,
		InternalSendCount: summary.InternalSendCount,
		Rows:              renderRows,
	})
	if err != nil {
		return buildReply(msg, fmt.Sprintf("统计图片生成失败: %v", err))
	}
	return buildImageReply(msg, imageBytes)
}

func (m *Module) handleGlobalStats(msg *core.OneBotMessage, r DateRange) map[string]interface{} {
	summary, err := m.store.QueryGlobalSummary(r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("统计查询失败: %v", err))
	}

	rows, err := m.store.QuerySessionTraffic(r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("全局统计查询失败: %v", err))
	}
	globalBotRows, err := m.store.QueryGlobalBotSend(r)
	if err != nil {
		return buildReply(msg, fmt.Sprintf("全局 bot 统计查询失败: %v", err))
	}
	if summary.RecvCount == 0 && summary.SendCount == 0 && len(rows) == 0 {
		return buildReply(msg, "该时间范围暂无统计记录")
	}

	botNames := collectGlobalBotNames(globalBotRows)
	groupIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.SessionType == "group" && row.SessionID > 0 {
			groupIDs = append(groupIDs, row.SessionID)
		}
	}
	groupAvatars := fetchGroupAvatars(groupIDs)
	renderRows := make([]GlobalStatsRow, 0, len(rows))
	for _, row := range rows {
		botCounts := make([]int64, len(botNames))
		sessionBotRows, err := m.store.QueryBotSend(row.SessionType, row.SessionID, r)
		if err != nil {
			return buildReply(msg, fmt.Sprintf("会话 bot 统计查询失败: %v", err))
		}
		sessionBotMap := make(map[string]int64, len(sessionBotRows))
		for _, botRow := range sessionBotRows {
			sessionBotMap[botRow.BotName] = botRow.SendCount
		}
		for i, botName := range botNames {
			botCounts[i] = sessionBotMap[botName]
		}
		renderRows = append(renderRows, GlobalStatsRow{
			SessionName: m.globalSessionDisplayName(row.SessionType, row.SessionID),
			TotalCount:  row.RecvCount + row.SendCount,
			BotCounts:   botCounts,
			Avatar:      groupAvatars[row.SessionID],
		})
	}

	imageBytes, err := RenderGlobalStatsImage(m.fontPath, RenderGlobalStatsInput{
		Title:             globalStatsTitle(r),
		SessionName:       "全部会话",
		RangeLabel:        rangeLabel(r),
		RecvCount:         summary.RecvCount,
		SendCount:         summary.SendCount,
		BotSendCount:      summary.BotSendCount,
		InternalSendCount: summary.InternalSendCount,
		BotNames:          botNames,
		BotSummary:        fillGlobalBotSummary(botNames, globalBotRows),
		Rows:              renderRows,
	})
	if err != nil {
		return buildReply(msg, fmt.Sprintf("统计图片生成失败: %v", err))
	}
	return buildImageReply(msg, imageBytes)
}

func collectGlobalBotNames(globalBotRows []BotSendRank) []string {
	seen := make(map[string]struct{})
	names := make([]string, 0, len(core.CONFIG.BotApps))
	for _, bot := range core.CONFIG.BotApps {
		names = append(names, bot.Name)
		seen[bot.Name] = struct{}{}
	}

	active := make(map[string]struct{})
	for _, botRow := range globalBotRows {
		if botRow.SendCount <= 0 {
			continue
		}
		active[botRow.BotName] = struct{}{}
		if _, ok := seen[botRow.BotName]; !ok {
			names = append(names, botRow.BotName)
			seen[botRow.BotName] = struct{}{}
		}
	}

	filtered := names[:0]
	for _, name := range names {
		if _, ok := active[name]; ok {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func fillGlobalBotSummary(botNames []string, rows []BotSendRank) []int64 {
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.BotName] = row.SendCount
	}
	result := make([]int64, len(botNames))
	for i, botName := range botNames {
		result[i] = counts[botName]
	}
	return result
}

func extractCommandText(msg *core.OneBotMessage) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Partial.MessageString) != "" {
		return strings.TrimSpace(msg.Partial.MessageString)
	}
	if strings.TrimSpace(msg.Partial.RawMessage) != "" {
		return strings.TrimSpace(msg.Partial.RawMessage)
	}
	return ""
}

func buildReply(msg *core.OneBotMessage, text string) map[string]interface{} {
	params := map[string]interface{}{"message": text}
	if msg.Partial.MessageType == "group" {
		params["group_id"] = msg.Partial.GroupID
		return map[string]interface{}{"action": "send_group_msg", "params": params}
	}
	params["user_id"] = core.GetMessageUserID(msg)
	return map[string]interface{}{"action": "send_private_msg", "params": params}
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
	if msg.Partial.MessageType == "group" {
		params["group_id"] = msg.Partial.GroupID
		return map[string]interface{}{"action": "send_group_msg", "params": params}
	}
	params["user_id"] = core.GetMessageUserID(msg)
	return map[string]interface{}{"action": "send_private_msg", "params": params}
}

func (m *Module) rankLimit() int {
	if m.cfg.DefaultRankLimit > 0 {
		return m.cfg.DefaultRankLimit
	}
	return 15
}

type syncNameResolver interface {
	ResolveGroupMemberNameSync(groupID, userID int64) (string, error)
	ResolvePrivateNameSync(userID int64) (string, error)
	ResolveGroupNameSync(groupID int64) (string, error)
}

func (m *Module) displayName(groupID, userID int64, snapshot string, isGroup bool) string {
	if m.resolver != nil {
		if isGroup {
			if syncResolver, ok := m.resolver.(syncNameResolver); ok {
				if name, err := syncResolver.ResolveGroupMemberNameSync(groupID, userID); err == nil && strings.TrimSpace(name) != "" {
					return name
				}
			}
			if name, err := m.resolver.ResolveGroupMemberName(groupID, userID); err == nil && strings.TrimSpace(name) != "" {
				return name
			}
		} else if name, err := m.resolver.ResolvePrivateName(userID); err == nil && strings.TrimSpace(name) != "" {
			return name
		} else if syncResolver, ok := m.resolver.(syncNameResolver); ok {
			if name, err := syncResolver.ResolvePrivateNameSync(userID); err == nil && strings.TrimSpace(name) != "" {
				return name
			}
		}
	}
	if strings.TrimSpace(snapshot) != "" {
		return snapshot
	}
	return strconv.FormatInt(userID, 10)
}

func (m *Module) isSuperUser(userID int64) bool {
	if m.superUser == nil {
		return false
	}
	return m.superUser(userID)
}

func sessionFromMessage(msg *core.OneBotMessage) (string, int64) {
	if msg.Partial.MessageType == "group" {
		return "group", msg.Partial.GroupID
	}
	return "private", 0
}

func (m *Module) messageSessionDisplayName(msg *core.OneBotMessage) string {
	if msg.Partial.MessageType == "group" {
		return m.globalSessionDisplayName("group", msg.Partial.GroupID)
	}
	return "私聊汇总"
}

func rangeLabel(r DateRange) string {
	if r.Mode == ModeAll {
		return "总计"
	}
	if r.StartDate == r.EndDate {
		return r.StartDate
	}
	return fmt.Sprintf("%s ~ %s", r.StartDate, r.EndDate)
}

func rankTitle(r DateRange) string {
	switch {
	case r.Mode == ModeAll:
		return "总发言榜"
	case r.Label == "今日":
		return "今日发言榜"
	case r.Label == "昨日":
		return "昨日发言榜"
	default:
		return "发言榜"
	}
}

func statsTitle(r DateRange) string {
	if r.Mode == ModeAll {
		return "总消息统计"
	}
	return "消息统计"
}

func globalStatsTitle(r DateRange) string {
	if r.Mode == ModeAll {
		return "全局总消息统计"
	}
	return "全局消息统计"
}

func (m *Module) globalSessionDisplayName(sessionType string, sessionID int64) string {
	if sessionType == "private" {
		if strings.TrimSpace(m.cfg.PrivateLabel) != "" {
			return m.cfg.PrivateLabel
		}
		return "私聊汇总"
	}
	if m.resolver != nil {
		if syncResolver, ok := m.resolver.(syncNameResolver); ok {
			if name, err := syncResolver.ResolveGroupNameSync(sessionID); err == nil && strings.TrimSpace(name) != "" {
				return fmt.Sprintf("%s(%d)", name, sessionID)
			}
		}
		if name, err := m.resolver.ResolveGroupName(sessionID); err == nil && strings.TrimSpace(name) != "" {
			return fmt.Sprintf("%s(%d)", name, sessionID)
		}
	}
	return fmt.Sprintf("群聊(%d)", sessionID)
}

func sessionAvatar(sessionType string, sessionID int64) []byte {
	if sessionType != "group" || sessionID <= 0 {
		return nil
	}
	return fetchGroupAvatar(sessionID)
}

func percentage(v, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(v) * 100 / float64(total)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
