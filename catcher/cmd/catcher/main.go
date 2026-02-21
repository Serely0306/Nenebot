package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"lunabot-catcher/internal/android"
	"lunabot-catcher/internal/config"
	"lunabot-catcher/internal/proxy"
	"lunabot-catcher/internal/uploader"
)

const banner = `
╔══════════════════════════════════════════════════════════════╗
║           LunaBot Catcher - 世界计划抓包工具                  ║
║                       v1.0.0                                 ║
╠══════════════════════════════════════════════════════════════╣
║  监听地址: %s
║  上传服务: %s
║  本地保存: %s
╠══════════════════════════════════════════════════════════════╣
║  启动游戏后，数据会自动抓取并上传                             ║
║  按 Ctrl+C 退出                                              ║
╚══════════════════════════════════════════════════════════════╝
`

func main() {
	// 命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 数据目录
	dataDir := filepath.Dir(*configPath)
	if dataDir == "" || dataDir == "." {
		dataDir, _ = os.Getwd()
	}

	// 创建 Android 辅助器
	androidHelper := android.NewAndroidHelper(dataDir)

	// 如果在 Android 上运行，进行初始化
	if androidHelper.IsAndroid() {
		log.Println("[系统] 检测到 Android 系统")

		if !androidHelper.IsRoot() {
			log.Println("[警告] 未检测到 Root 权限，部分功能可能不可用")
		} else {
			// 设置 DNS
			if err := androidHelper.SetupDNS(); err != nil {
				log.Printf("[警告] DNS 设置失败: %v", err)
			}
		}
	}

	// 创建上传器
	up := uploader.NewUploader(cfg.UploadServer, cfg.SaveLocally, cfg.SaveDir)

	// 创建代理
	mitmProxy, err := proxy.NewMitmProxy(up, cfg, dataDir)
	if err != nil {
		log.Fatalf("创建代理失败: %v", err)
	}

	// Android 特定操作
	if androidHelper.IsAndroid() && androidHelper.IsRoot() {
		// 安装 CA 证书
		installed, err := androidHelper.InstallCert(mitmProxy.GetCertPath())
		if err != nil {
			log.Printf("[警告] 安装证书失败: %v", err)
		} else if installed {
			// 新安装了证书，需要重启设备才能生效
			if err := androidHelper.Reboot(); err != nil {
				log.Printf("[警告] 自动重启失败: %v，请手动重启设备", err)
			}
			os.Exit(0)
		}

		// 获取代理 IP
		proxyIP := cfg.AndroidProxyIP
		if proxyIP == "" {
			proxyIP = androidHelper.GetLocalIP()
		}

		// 解析端口
		var port int
		fmt.Sscanf(cfg.Listen, "%*[^:]:%d", &port)
		if port == 0 {
			port = 8888
		}

		// 设置系统代理
		if err := androidHelper.SetProxy(proxyIP, port); err != nil {
			log.Printf("[警告] 设置代理失败: %v", err)
		}
	}

	// 打印 banner
	saveStatus := "否"
	if cfg.SaveLocally {
		saveStatus = "是"
	}
	fmt.Printf(banner, cfg.Listen, cfg.UploadServer, saveStatus)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n正在退出...")

		// 清理 Android 代理设置
		if androidHelper.IsAndroid() && androidHelper.IsRoot() {
			androidHelper.ClearProxy()
		}

		os.Exit(0)
	}()

	// 启动代理
	if err := mitmProxy.ListenAndServe(cfg.Listen); err != nil {
		log.Fatalf("启动代理失败: %v", err)
	}
}
