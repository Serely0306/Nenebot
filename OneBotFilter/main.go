package main

import (
	"fmt"
	"log"
	"net/http"
	filter "onebotfilter/src"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wss = &filter.WsServer{}
)

// checkAccessToken 验证连接的access-token
// 支持两种传递方式（OneBot v11标准）：
// 1. Authorization: Bearer <token> header
// 2. ?access_token=<token> query parameter
func checkAccessToken(r *http.Request) bool {
	expected := filter.CONFIG.Server.AccessToken
	if expected == "" {
		return true // 未配置token则不验证
	}

	// 方式1：从 Authorization header 获取
	auth := r.Header.Get("Authorization")
	if auth != "" {
		token := strings.TrimPrefix(auth, "Bearer ")
		token = strings.TrimPrefix(token, "Token ")
		if strings.TrimSpace(token) == expected {
			return true
		}
	}

	// 方式2：从 query parameter 获取
	queryToken := r.URL.Query().Get("access_token")
	if queryToken == expected {
		return true
	}

	return false
}

// handleFileServer 处理文件服务请求
// 验证access-token和路径安全性后提供文件下载
func handleFileServer(w http.ResponseWriter, r *http.Request) {
	// 验证access-token
	if !checkAccessToken(r) {
		http.Error(w, "access-token验证失败", http.StatusUnauthorized)
		return
	}

	cfg := filter.CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" {
		http.Error(w, "文件服务未启用", http.StatusNotFound)
		return
	}

	// 提取请求的相对路径
	relPath := strings.TrimPrefix(r.URL.Path, "/files/")
	if relPath == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}

	// 构建绝对路径并进行安全检查
	root := filepath.Clean(cfg.Root)
	fullPath := filepath.Join(root, filepath.Clean("/"+relPath))
	fullPath = filepath.Clean(fullPath)

	// 防止路径穿越攻击
	if !strings.HasPrefix(fullPath, root) {
		log.Printf("文件服务：路径穿越攻击被阻止: %s\n", r.URL.Path)
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}

	if filter.CONFIG.Server.Debug {
		log.Printf("文件服务：提供文件: %s\n", fullPath)
	}

	// 提供文件
	http.ServeFile(w, r, fullPath)
}

func handleLocal(w http.ResponseWriter, r *http.Request) {
	// 验证access-token
	if !checkAccessToken(r) {
		log.Println("OneBot客户端连接被拒绝：access-token验证失败")
		http.Error(w, "access-token验证失败", http.StatusUnauthorized)
		return
	}

	if wss.Conn != nil {
		http.Error(w, "只能连接一个OneBot客户端", http.StatusForbidden)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
		return
	}
	// defer conn.Close()
	wss.Conn = conn

	// 自动从NapCat/LLBot的请求头中获取bot ID
	var newBotId string
	selfId := strings.TrimSpace(r.Header.Get("X-Self-ID"))

	if selfId != "" {
		newBotId = selfId
		log.Printf("已自动识别Bot ID: %s\n", selfId)
	} else if filter.CONFIG.Server.BotId != "" {
		newBotId = filter.CONFIG.Server.BotId
		log.Printf("使用配置中的Bot ID: %s\n", newBotId)
	} else {
		log.Println("警告：未能获取Bot ID，请在config.yaml中配置bot-id")
	}

	// 检查 Bot ID 是否发生变化
	if newBotId != "" {
		if wss.BotId != "" && wss.BotId != newBotId {
			log.Printf("检测到Bot ID变更 (%s -> %s)，正在重置所有连接...\n", wss.BotId, newBotId)
			// 断开所有现有连接，迫使它们重新连接并使用新的 ID
			wss.DisconnectAllClients()
		}
		wss.BotId = newBotId
		filter.CONFIG.Server.BotId = newBotId
	}

	log.Println("已连接到OneBot客户端")
	err = wss.WsServerHandler()
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
	}
	//循环结束
	log.Println("OneBot客户端连接已断开")
}

func main() {
	err := filter.LoadConfigVP("config.yaml")
	if err != nil {
		log.Fatal("加载配置异常:", err)
	}
	upgrader.ReadBufferSize = filter.CONFIG.Server.BufferSize
	upgrader.WriteBufferSize = filter.CONFIG.Server.BufferSize
	http.HandleFunc(filter.CONFIG.Server.Suffix, handleLocal)

	// 注册文件服务
	if filter.CONFIG.Server.FileServer.Enabled {
		http.HandleFunc("/files/", handleFileServer)
		log.Printf("文件服务已启用 root: %s, public-url: %s\n",
			filter.CONFIG.Server.FileServer.Root,
			filter.CONFIG.Server.FileServer.PublicURL)
	}

	go func() {
		for _, bacfg := range filter.CONFIG.BotApps {
			go filter.WsClientHandler(wss, bacfg)
		}
	}()

	log.Printf("OneBotFilter已启动 ws://%s:%d%s\n", filter.CONFIG.Server.Host, filter.CONFIG.Server.Port, filter.CONFIG.Server.Suffix)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", filter.CONFIG.Server.Host, filter.CONFIG.Server.Port), nil); err != nil {
		log.Fatal("监听服务出错:", err)
	}
}
