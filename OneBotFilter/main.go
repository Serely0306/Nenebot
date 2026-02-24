package main

import (
	"fmt"
	"log"
	"net/http"
	filter "onebotfilter/src"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	wss         = &filter.WsServer{}
	configBotId string // 保存配置文件中的原始 BotId，不被自动检测覆盖
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
// 支持路径内嵌 token 格式: /files/{token}/relpath
// 也支持无 token 格式: /files/relpath (此时通过 query parameter 或 header 验证)
func handleFileServer(w http.ResponseWriter, r *http.Request) {
	cfg := filter.CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" {
		http.Error(w, "文件服务未启用", http.StatusNotFound)
		return
	}

	// 提取 /files/ 后面的路径
	remainPath := strings.TrimPrefix(r.URL.Path, "/files/")
	if remainPath == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}

	expectedToken := filter.CONFIG.Server.AccessToken
	var relPath string

	if expectedToken != "" {
		// 尝试从路径中提取 token: /files/{token}/relpath
		slashIdx := strings.Index(remainPath, "/")
		if slashIdx > 0 {
			pathToken := remainPath[:slashIdx]
			if pathToken == expectedToken {
				// token 匹配，提取相对路径
				relPath = remainPath[slashIdx+1:]
			}
		}

		// 如果路径中没有 token，回退到 query parameter / header 验证
		if relPath == "" {
			if !checkAccessToken(r) {
				http.Error(w, "access-token验证失败", http.StatusUnauthorized)
				return
			}
			relPath = remainPath
		}
	} else {
		// 未配置 token，直接使用路径
		relPath = remainPath
	}

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

	// 如果旧连接存在，主动关闭旧连接让新连接接入
	if wss.Conn != nil {
		log.Println("检测到旧连接仍存在，正在关闭旧连接以接受新连接...")
		wss.Conn.Close()
		// 等待旧的 WsServerHandler 退出并清理完成
		for i := 0; i < 50 && wss.Conn != nil; i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if wss.Conn != nil {
			log.Println("警告：旧连接清理超时，强制重置")
			wss.Conn = nil
		}
		log.Println("旧连接已清理完毕")
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
		return
	}
	// defer conn.Close()
	wss.Conn = conn

	// 确定 Bot ID：配置文件中的 bot-id 优先，否则从 NapCat 请求头自动检测
	var newBotId string
	selfId := strings.TrimSpace(r.Header.Get("X-Self-ID"))

	if configBotId != "" {
		// 配置文件明确指定了 bot-id，始终使用它（忽略 NapCat 可能发来的错误 ID）
		newBotId = configBotId
		if selfId != "" && selfId != configBotId {
			log.Printf("注意：NapCat 发送的 X-Self-ID (%s) 与配置的 bot-id (%s) 不同，使用配置值\n", selfId, configBotId)
		} else {
			log.Printf("使用配置中的Bot ID: %s\n", newBotId)
		}
	} else if selfId != "" {
		// 未配置 bot-id，从 NapCat 自动检测
		newBotId = selfId
		log.Printf("已自动识别Bot ID: %s\n", selfId)
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
	configBotId = strings.TrimSpace(filter.CONFIG.Server.BotId) // 保存配置中的原始 BotId
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
