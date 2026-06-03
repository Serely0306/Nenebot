package apply

import (
	"encoding/json"
	"html"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"onebotfilter/src/core"
)

var cqJSONRe = regexp.MustCompile(`\[CQ:json,data=([^\]]+)\]`)

type Config struct {
	Enabled                bool   `yaml:"enabled"`
	ApplicationsPath       string `yaml:"applications_path"`
	AutoApproveFriend      bool   `yaml:"auto_approve_friend"`
	AutoApproveGroupInvite bool   `yaml:"auto_approve_group_invite"`
	RefreshSeconds         int    `yaml:"refresh_seconds"`
	ReviewURL              string `yaml:"review_url"`
	FontPath               string `yaml:"font_path"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:                false,
		ApplicationsPath:       "../upload/data/applications.json",
		AutoApproveFriend:      true,
		AutoApproveGroupInvite: true,
		RefreshSeconds:         30,
		ReviewURL:              "",
	}
}

type Module struct {
	cfg       Config
	whitelist *Whitelist
	wss       *core.WsServer
	stopCh    chan struct{}
	fontPath  string
}

func New(cfg Config, wss *core.WsServer, fontPath string) *Module {
	if cfg.RefreshSeconds <= 0 {
		cfg.RefreshSeconds = 30
	}
	m := &Module{
		cfg:       cfg,
		whitelist: NewWhitelist(cfg.ApplicationsPath),
		wss:       wss,
		stopCh:    make(chan struct{}),
		fontPath:  fontPath,
	}
	if cfg.Enabled {
		go m.refreshLoop()
	}
	return m
}

func (m *Module) Stop() {
	close(m.stopCh)
}

// HandleUpstreamEvent 拦截 request/notice 事件及私聊消息中的群邀请链接
func (m *Module) HandleUpstreamEvent(msg *core.OneBotMessage) {
	if !m.cfg.Enabled || msg == nil {
		return
	}

	postType := extractString(msg.Intact, "post_type")

	// 路径 A: request 事件 — LLOneBot 标准 group_request
	// post_type="request", request_type="group", sub_type="invite"|"add"
	if postType == "request" {
		m.handleRequest(msg)
		return
	}

	// 路径 B: notice 事件 — 部分实现可能用此格式，兼容
	if postType == "notice" {
		m.handleNotice(msg)
		return
	}

	// 路径 C: 私聊消息中的群邀请链接 — QQ 新版/降级路径
	if postType == "message" {
		m.handleMessage(msg)
	}
}

// ── 路径 A: request 事件（LLOneBot 标准格式）──

func (m *Module) handleRequest(msg *core.OneBotMessage) {
	requestType := extractString(msg.Intact, "request_type")
	if requestType != "group" {
		// friend 请求
		if requestType == "friend" && m.cfg.AutoApproveFriend {
			m.handleFriendRequestNotice(msg)
		}
		return
	}

	if !m.cfg.AutoApproveGroupInvite {
		return
	}

	subType := extractString(msg.Intact, "sub_type")
	if subType != "invite" {
		return
	}

	groupID := extractString(msg.Intact, "group_id")
	userID := extractString(msg.Intact, "user_id")
	flag := extractString(msg.Intact, "flag")

	if flag == "" {
		log.Println("[apply] request/group/invite 缺少 flag，跳过")
		return
	}

	if !m.whitelist.Contains(userID) {
		log.Printf("[apply] 群邀请发送者不在白名单: user=%s group=%s\n", userID, groupID)
		return
	}

	log.Printf("[apply] 检测到群邀请事件(request): group=%s user=%s\n", groupID, userID)
	m.doApproveGroupInvite(flag, userID, groupID)
}

// ── 路径 B: notice 事件（兼容部分旧版实现）──

func (m *Module) handleNotice(msg *core.OneBotMessage) {
	noticeType := extractString(msg.Intact, "notice_type")

	switch noticeType {
	case "friend_request":
		if !m.cfg.AutoApproveFriend {
			return
		}
		m.handleFriendRequestNotice(msg)
	case "group_request":
		if !m.cfg.AutoApproveGroupInvite {
			return
		}
		subType := extractString(msg.Intact, "sub_type")
		if subType == "invite" {
			userID := extractString(msg.Intact, "user_id")
			groupID := extractString(msg.Intact, "group_id")
			flag := extractString(msg.Intact, "flag")
			if flag == "" {
				log.Println("[apply] notice/group_request/invite 缺少 flag，跳过")
				return
			}
			if !m.whitelist.Contains(userID) {
				return
			}
			log.Printf("[apply] 检测到群邀请事件(notice): group=%s user=%s\n", groupID, userID)
			m.doApproveGroupInvite(flag, userID, groupID)
		}
	}
}

// ── 路径 C: 私聊消息群邀请链接 ──

type arkInviteData struct {
	App    string `json:"app"`
	BizSrc string `json:"bizsrc"`
	Meta   struct {
		News struct {
			JumpURL string `json:"jumpUrl"`
			Desc    string `json:"desc"`
		} `json:"news"`
	} `json:"meta"`
}

func (m *Module) handleMessage(msg *core.OneBotMessage) {
	if !m.cfg.AutoApproveGroupInvite {
		return
	}
	if msg.Partial.MessageType != "private" {
		return
	}

	raw := msg.Partial.RawMessage
	matches := cqJSONRe.FindStringSubmatch(raw)
	if len(matches) < 2 {
		return
	}

	jsonStr := html.UnescapeString(matches[1])
	var ark arkInviteData
	if err := json.Unmarshal([]byte(jsonStr), &ark); err != nil {
		return
	}

	if ark.App != "com.tencent.tuwen.lua" && ark.App != "com.tencent.qun.invite" {
		return
	}
	if ark.App == "com.tencent.tuwen.lua" && ark.BizSrc != "qun.invite" {
		return
	}

	jumpURL := ark.Meta.News.JumpURL
	if jumpURL == "" {
		return
	}

	u, err := url.Parse(jumpURL)
	if err != nil {
		log.Printf("[apply] 解析群邀请 jumpUrl 失败: %v\n", err)
		return
	}
	q := u.Query()

	userID := q.Get("senderuin")
	groupCode := q.Get("groupcode")
	msgSeq := q.Get("msgseq")

	if userID == "" || groupCode == "" || msgSeq == "" {
		log.Printf("[apply] 群邀请链接缺少必要参数: user=%s group=%s seq=%s\n", userID, groupCode, msgSeq)
		return
	}

	if !m.whitelist.Contains(userID) {
		log.Printf("[apply] 群邀请发送者不在白名单: user=%s group=%s\n", userID, groupCode)
		return
	}

	// flag 格式来自 LLOneBot 源码: ${groupCode}|${msgseq}|1|0
	flag := groupCode + "|" + msgSeq + "|1|0"

	log.Printf("[apply] 检测到私聊群邀请链接: group=%s user=%s flag=%s\n", groupCode, userID, flag)
	m.doApproveGroupInvite(flag, userID, groupCode)
}

// ── 好友请求处理（request + notice 共用）──

func (m *Module) handleFriendRequestNotice(msg *core.OneBotMessage) {
	userID := extractString(msg.Intact, "user_id")
	flag := extractString(msg.Intact, "flag")
	if flag == "" {
		log.Println("[apply] 好友请求缺少 flag，跳过")
		return
	}
	if !m.whitelist.Contains(userID) {
		return
	}
	log.Printf("[apply] 自动通过好友请求: user=%s\n", userID)
	_, err := m.wss.CallAPI("handle_friend_request", map[string]interface{}{
		"flag":    flag,
		"approve": true,
	}, 5*time.Second)
	if err != nil {
		log.Printf("[apply] 处理好友请求失败: user=%s err=%v\n", userID, err)
	}
}

// ── 通用群邀请通过 ──

func (m *Module) doApproveGroupInvite(flag, userID, groupID string) {
	log.Printf("[apply] 自动通过群邀请: group=%s user=%s\n", groupID, userID)
	_, err := m.wss.CallAPI("handle_group_request", map[string]interface{}{
		"flag":     flag,
		"sub_type": "invite",
		"approve":  true,
	}, 5*time.Second)
	if err != nil {
		log.Printf("[apply] handle_group_request 失败: %v，尝试 set_group_add_request...\n", err)
		_, err = m.wss.CallAPI("set_group_add_request", map[string]interface{}{
			"flag":     flag,
			"sub_type": "invite",
			"approve":  true,
		}, 5*time.Second)
		if err != nil {
			log.Printf("[apply] set_group_add_request 也失败: group=%s user=%s err=%v\n", groupID, userID, err)
		} else {
			log.Printf("[apply] 已通过群邀请(set_group_add_request): group=%s user=%s\n", groupID, userID)
		}
	} else {
		log.Printf("[apply] 已通过群邀请(handle_group_request): group=%s user=%s\n", groupID, userID)
	}
}

// ── 白名单刷新 ──

func (m *Module) refreshLoop() {
	interval := time.Duration(m.cfg.RefreshSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.whitelist.Refresh()
		case <-m.stopCh:
			return
		}
	}
}

// ── 工具 ──

func extractString(intact map[string]json.RawMessage, key string) string {
	raw, ok := intact[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n int64
		if err2 := json.Unmarshal(raw, &n); err2 == nil {
			return strconv.FormatInt(n, 10)
		}
		var f float64
		if err2 := json.Unmarshal(raw, &f); err2 == nil {
			return strconv.FormatInt(int64(f), 10)
		}
		return ""
	}
	return strings.TrimSpace(s)
}
