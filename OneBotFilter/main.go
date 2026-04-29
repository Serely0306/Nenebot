package main

import (
	"fmt"
	"log"
	"net/http"
	"onebotfilter/src/core"
	filtermod "onebotfilter/src/filter"
	helpmod "onebotfilter/src/help"
	statsmod "onebotfilter/src/stats"
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
	wss         = &core.WsServer{}
	configBotId string
)

func checkAccessToken(r *http.Request) bool {
	expected := core.CONFIG.Server.AccessToken
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
	cfg := core.CONFIG.Server.FileServer
	if !cfg.Enabled || cfg.Root == "" {
		http.Error(w, "文件服务未启用", http.StatusNotFound)
		return
	}

	remainPath := strings.TrimPrefix(r.URL.Path, "/files/")
	if remainPath == "" {
		http.Error(w, "未指定文件路径", http.StatusBadRequest)
		return
	}

	expectedToken := core.CONFIG.Server.AccessToken
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

	if core.CONFIG.Server.Debug {
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

	selfID := core.NormalizeBotID(r.Header.Get("X-Self-ID"))
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
		log.Println("警告：未能获取 Bot ID，请在 config/filter/config.yaml 中配置 bot-id")
	}

	if newBotID != "" {
		if wss.BotID != "" && wss.BotID != newBotID {
			log.Printf("检测到 Bot ID 变更 (%s -> %s)，正在重置所有连接...\n", wss.BotID, newBotID)
			wss.DisconnectAllClients()
		}
		wss.BotID = newBotID
	}

	log.Println("已连接到 OneBot 客户端")
	err = wss.WsServerHandler()
	if err != nil {
		log.Println("OneBot客户端连接异常：", err)
	}
	log.Println("OneBot客户端连接已断开")
}

type oneBotNameResolver struct {
	server *core.WsServer
}

func (r oneBotNameResolver) ResolveGroupMemberName(groupID, userID int64) (string, error) {
	if r.server == nil || r.server.Conn == nil {
		return "", fmt.Errorf("onebot 未连接")
	}

	resp, err := r.server.CallAPI("get_group_member_info", map[string]interface{}{
		"group_id": groupID,
		"user_id":  userID,
		"no_cache": true,
	}, 3*time.Second)
	if err != nil {
		return "", err
	}
	data, _ := resp["data"].(map[string]interface{})
	card, _ := data["card"].(string)
	if strings.TrimSpace(card) != "" {
		return strings.TrimSpace(card), nil
	}
	nickname, _ := data["nickname"].(string)
	return strings.TrimSpace(nickname), nil
}

func (r oneBotNameResolver) ResolvePrivateName(userID int64) (string, error) {
	if r.server == nil || r.server.Conn == nil {
		return "", fmt.Errorf("onebot 未连接")
	}

	resp, err := r.server.CallAPI("get_stranger_info", map[string]interface{}{
		"user_id":  userID,
		"no_cache": true,
	}, 3*time.Second)
	if err != nil {
		return "", err
	}
	data, _ := resp["data"].(map[string]interface{})
	nickname, _ := data["nickname"].(string)
	return strings.TrimSpace(nickname), nil
}

type moduleFilter struct {
	module  *filtermod.Module
	botName string
}

func (f moduleFilter) Filter(msg *core.OneBotMessage) bool {
	return f.module.FilterBotMessage(f.botName, msg)
}

func (f moduleFilter) String() string {
	return f.botName
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal("获取工作目录失败:", err)
	}

	paths := core.DefaultPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.HelpImage), 0o755); err != nil {
		log.Fatal("创建 data/help 目录失败:", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.StatsDB), 0o755); err != nil {
		log.Fatal("创建 data/stats 目录失败:", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.StatsConfig), 0o755); err != nil {
		log.Fatal("创建 config/stats 目录失败:", err)
	}

	core.SetConfigResolver(filtermod.ResolveConfig)
	err = core.LoadConfigVP(paths.FilterConfig)
	if err != nil {
		log.Fatal("加载配置异常:", err)
	}
	helpSettings := helpmod.Settings{
		Enabled:         true,
		GenerateImage:   core.CONFIG.Server.Help.Generate,
		ForwardNickname: core.CONFIG.Server.Help.ForwardNickname,
		BotID:           core.NormalizeBotID(core.CONFIG.Server.BotID),
	}
	helpModule, err := helpmod.Load(paths.HelpConfig, paths, helpSettings)
	if err != nil {
		log.Fatal("加载帮助配置异常:", err)
	}

	statsCfg, err := statsmod.LoadConfig(paths.StatsConfig)
	if err != nil {
		log.Fatal("加载统计配置异常:", err)
	}
	statsStore, err := statsmod.OpenStore(paths.StatsDB)
	if err != nil {
		log.Fatal("打开统计数据库异常:", err)
	}
	defer statsStore.Close()
	statsModule := statsmod.NewModule(statsCfg, statsStore, oneBotNameResolver{server: wss}, core.IsSuperUser, paths.FontFile)
	statsModule.Start()
	defer statsModule.Stop()

	filterModule := filtermod.NewModule(nil)
	if err := filterModule.Reload(); err != nil {
		log.Fatal("初始化过滤器失败:", err)
	}
	core.SetConfigReloadHook(func() {
		if err := filterModule.Reload(); err != nil {
			log.Printf("重新加载过滤器失败: %v\n", err)
		}
	})
	wss.SetExternalCommandHandlers(
		helpModule.TryHandle,
		statsModule.TryHandle,
		filterModule.TryHandle,
	)
	wss.SetUpstreamEventHooks(statsModule.HandleUpstreamEvent)
	wss.SetBotActionHooks(statsModule.HandleBotAction)
	wss.SetInternalSendHooks(statsModule.HandleInternalSend)

	// 命令行子命令: generate-help
	if len(os.Args) > 1 && os.Args[1] == "generate-help" {
		outputPath := paths.HelpImage
		if len(os.Args) > 2 {
			outputPath = os.Args[2]
		}
		if err := helpmod.SaveImage(helpModule.Config(), paths.FontFile, outputPath); err != nil {
			log.Fatalf("生成帮助图片失败: %v\n", err)
		}
		log.Printf("帮助图片已生成: %s\n", outputPath)
		return
	}

	configBotId = core.NormalizeBotID(core.CONFIG.Server.BotID)
	upgrader.ReadBufferSize = core.CONFIG.Server.BufferSize
	upgrader.WriteBufferSize = core.CONFIG.Server.BufferSize
	http.HandleFunc(core.CONFIG.Server.Suffix, handleLocal)

	if core.CONFIG.Server.FileServer.Enabled {
		http.HandleFunc("/files/", handleFileServer)
		log.Printf("文件服务已启用 root=%s public-url=%s\n",
			core.CONFIG.Server.FileServer.Root,
			core.CONFIG.Server.FileServer.PublicURL)
	}

	go func() {
		for _, bacfg := range core.CONFIG.BotApps {
			go core.WsClientHandler(wss, bacfg, moduleFilter{module: filterModule, botName: bacfg.Name})
		}
	}()

	log.Printf("OneBotFilter已启动 ws://%s:%d%s\n", core.CONFIG.Server.Host, core.CONFIG.Server.Port, core.CONFIG.Server.Suffix)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", core.CONFIG.Server.Host, core.CONFIG.Server.Port), nil); err != nil {
		log.Fatal("监听服务出错:", err)
	}
}
