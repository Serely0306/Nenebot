package main

import (
	"fmt"
	"log"
	"net/http"
	filter "onebotfilter/src"
	"os"
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
	configBotId string
)

func checkAccessToken(r *http.Request) bool {
	expected := filter.CONFIG.Server.AccessToken
	if expected == "" {
		return true
	}

	auth := r.Header.Get("Authorization")
	if auth != "" {
		token := strings.TrimPrefix(auth, "Bearer ")
		token = strings.TrimPrefix(token, "Token ")
		if strings.TrimSpace(token) == expected {
			return true
		}
	}

	queryToken := r.URL.Query().Get("access_token")
	if queryToken == expected {
		return true
	}

	return false
}

func handleFileServer(w http.ResponseWriter, r *http.Request) {
	cfg := filter.CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" {
		http.Error(w, "文件服务未启用", http.StatusNotFound)
		return
	}

	remainPath := strings.TrimPrefix(r.URL.Path, "/files/")
	if remainPath == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}

	expectedToken := filter.CONFIG.Server.AccessToken
	var relPath string

	if expectedToken != "" {
		slashIdx := strings.Index(remainPath, "/")
		if slashIdx > 0 {
			pathToken := remainPath[:slashIdx]
			if pathToken == expectedToken {
				relPath = remainPath[slashIdx+1:]
			}
		}

		if relPath == "" {
			if !checkAccessToken(r) {
				http.Error(w, "access-token验证失败", http.StatusUnauthorized)
				return
			}
			relPath = remainPath
		}
	} else {
		relPath = remainPath
	}

	if relPath == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}

	root := filepath.Clean(cfg.Root)
	fullPath := filepath.Join(root, filepath.Clean("/"+relPath))
	fullPath = filepath.Clean(fullPath)

	if !strings.HasPrefix(fullPath, root) {
		log.Printf("文件服务：路径穿越攻击已拦截: %s\n", r.URL.Path)
		http.Error(w, "禁止访问", http.StatusForbidden)
		return
	}

	if filter.CONFIG.Server.Debug {
		log.Printf("文件服务：提供文件 %s\n", fullPath)
	}

	http.ServeFile(w, r, fullPath)
}

func handleLocal(w http.ResponseWriter, r *http.Request) {
	if !checkAccessToken(r) {
		log.Println("OneBot客户端连接被拒绝：access-token验证失败")
		http.Error(w, "access-token验证失败", http.StatusUnauthorized)
		return
	}

	selfID := strings.TrimSpace(r.Header.Get("X-Self-ID"))
	if configBotId != "" && selfID != "" && selfID != configBotId {
		log.Printf("拒绝连接：X-Self-ID (%s) 与配置的 bot-id (%s) 不匹配\n", selfID, configBotId)
		http.Error(w, "Bot ID不匹配", http.StatusForbidden)
		return
	}

	if wss.Conn != nil {
		log.Println("检测到旧连接仍存在，正在关闭旧连接以接受新连接...")
		wss.Conn.Close()
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
	wss.Conn = conn

	var newBotID string
	if configBotId != "" {
		newBotID = configBotId
		log.Printf("使用配置中的 Bot ID: %s\n", newBotID)
	} else if selfID != "" {
		newBotID = selfID
		log.Printf("已自动识别 Bot ID: %s\n", selfID)
	} else {
		log.Println("警告：未能获取 Bot ID，请在 config.yaml 中配置 bot-id")
	}

	if newBotID != "" {
		if wss.BotId != "" && wss.BotId != newBotID {
			log.Printf("检测到 Bot ID 变更 (%s -> %s)，正在重置所有连接...\n", wss.BotId, newBotID)
			wss.DisconnectAllClients()
		}
		wss.BotId = newBotID
	}

	log.Println("已连接到 OneBot 客户端")
	err = wss.WsServerHandler()
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
	}
	log.Println("OneBot客户端连接已断开")
}

func main() {
	err := filter.LoadConfigVP("config.yaml")
	if err != nil {
		log.Fatal("加载配置异常:", err)
	}
	err = filter.LoadHelpConfig("help.yaml")
	if err != nil {
		log.Fatal("加载帮助配置异常:", err)
	}

	// 命令行子命令: generate-help
	if len(os.Args) > 1 && os.Args[1] == "generate-help" {
		outputPath := "help.png"
		if len(os.Args) > 2 {
			outputPath = os.Args[2]
		}
		if err := filter.GenerateHelpImageFromConfig(outputPath); err != nil {
			log.Fatalf("生成帮助图片失败: %v\n", err)
		}
		log.Printf("帮助图片已生成: %s\n", outputPath)
		return
	}

	configBotId = strings.TrimSpace(filter.CONFIG.Server.BotId)
	upgrader.ReadBufferSize = filter.CONFIG.Server.BufferSize
	upgrader.WriteBufferSize = filter.CONFIG.Server.BufferSize
	http.HandleFunc(filter.CONFIG.Server.Suffix, handleLocal)

	if filter.CONFIG.Server.FileServer.Enabled {
		http.HandleFunc("/files/", handleFileServer)
		log.Printf("文件服务已启用 root=%s public-url=%s\n",
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

