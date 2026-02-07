package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	config     *Config
	cfClient   *CloudflareClient
	ipChecker  *IPChecker
	running    bool
	currentIP  string
	reloadChan chan bool
)

func main() {
	// 解析命令行参数
	daemonMode := flag.Bool("daemon", false, "后台运行模式，直接开始监控（适合系统服务）")
	onceMode := flag.Bool("once", false, "执行一次更新后退出（适合 cron）")
	logFile := flag.Bool("log-file", false, "启用日志文件（daemon 模式默认启用）")
	stopFlag := flag.Bool("stop", false, "停止后台运行的守护进程")
	killFlag := flag.Bool("kill", false, "强制终止守护进程")
	statusFlag := flag.Bool("status", false, "查看守护进程状态")
	listFlag := flag.Bool("list", false, "列出所有dns_manager进程")
	infoFlag := flag.Bool("info", false, "查看守护进程详细信息")
	cleanupFlag := flag.Bool("cleanup", false, "清理无效的PID文件")
	manageFlag := flag.Bool("manage", false, "进入守护进程管理菜单")
	flag.Parse()

	// 列出所有进程
	if *listFlag {
		processes, err := listDaemonProcesses()
		if err != nil {
			fmt.Fprintf(os.Stderr, "列出进程失败: %v\n", err)
			os.Exit(1)
		}
		if len(processes) == 0 {
			fmt.Println("未找到运行中的 dns_manager 进程")
		} else {
			fmt.Printf("找到 %d 个 dns_manager 进程:\n", len(processes))
			fmt.Println(strings.Repeat("-", 80))
			for _, proc := range processes {
				fmt.Printf("PID: %d\n%s\n", proc.PID, proc.Command)
				fmt.Println(strings.Repeat("-", 80))
			}
		}
		os.Exit(0)
	}

	// 查看详细信息
	if *infoFlag {
		info, err := getDaemonInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取信息失败: %v\n", err)
			os.Exit(1)
		}
		printDaemonInfo(info)
		os.Exit(0)
	}

	// 清理PID文件
	if *cleanupFlag {
		if err := cleanupPIDFile(); err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("PID文件检查完成，无需清理")
		}
		os.Exit(0)
	}

	// 强制终止守护进程
	if *killFlag {
		if err := killDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "强制终止守护进程失败: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 停止守护进程
	if *stopFlag {
		if err := stopDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "停止守护进程失败: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// 查看状态
	if *statusFlag {
		pid, err := getPID()
		if err != nil {
			fmt.Println("守护进程未运行（未找到PID文件）")
			os.Exit(0)
		}
		if isProcessRunning(pid) {
			fmt.Printf("守护进程正在运行，PID: %d\n", pid)
		} else {
			fmt.Printf("守护进程未运行（PID文件存在但进程不存在，PID: %d）\n", pid)
			fmt.Println("提示: 使用 --cleanup 清理无效的PID文件")
		}
		os.Exit(0)
	}

	// 管理菜单
	if *manageFlag {
		manageDaemonMenu()
		os.Exit(0)
	}

	// 初始化日志（daemon 模式默认启用文件日志）
	enableFileLog := *logFile || *daemonMode
	if err := initLogger(enableFileLog, !*daemonMode); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer globalLogger.Close()

	// 初始化重载通道
	reloadChan = make(chan bool, 1)

	// 加载配置
	config = LoadConfig()
	if config.APIToken == "" || config.ZoneID == "" || config.RecordName == "" {
		logInfo("检测到未配置，请先进行配置...")
		interactiveConfig()
		config = LoadConfig()
	}

	// 初始化客户端
	var err error
	cfClient, err = NewCloudflareClient(config.APIToken)
	if err != nil {
		logError("初始化 Cloudflare 客户端失败: %v", err)
		os.Exit(1)
	}

	ipChecker = NewIPChecker()

	// 根据参数选择运行模式
	if *onceMode {
		// 执行一次模式
		runOnce()
		os.Exit(0)
	} else if *daemonMode {
		// 后台运行模式：自动daemon化
		// 如果不是守护进程，先转换为守护进程
		if os.Getppid() != 1 {
			if err := daemonize(); err != nil {
				fmt.Fprintf(os.Stderr, "守护进程化失败: %v\n", err)
				os.Exit(1)
			}
			// daemonize 会退出父进程，这里不会执行到
			return
		}
		// 已经是守护进程，直接运行
		runDaemon()
	} else {
		// 交互式模式（默认）
		// 如果配置已存在，提示可以自动启动
		if config.APIToken != "" && config.ZoneID != "" && config.RecordName != "" {
			fmt.Println("\n提示: 配置已存在，可以使用以下命令自动后台运行：")
			fmt.Println("  ./dns_manager --daemon")
			fmt.Println("  或使用交互式菜单选择 '1. 开始监控'")
			fmt.Println()
		}
		runInteractive()
	}
}

// 交互式模式
func runInteractive() {
	fmt.Println("=== Cloudflare DNS 动态更新系统 ===")
	fmt.Println()

	// 显示主菜单
	for {
		showMainMenu()
		choice := getUserInput("请选择操作 (1-8): ")

		switch choice {
		case "1":
			startMonitoring()
		case "2":
			checkCurrentIP()
		case "3":
			updateDNSNow()
		case "4":
			viewDNSRecords()
		case "5":
			interactiveConfig()
			config = LoadConfig()
			cfClient, _ = NewCloudflareClient(config.APIToken)
		case "6":
			startBackgroundDaemon()
		case "7":
			manageDaemonMenu()
		case "8":
			fmt.Println("感谢使用，再见！")
			os.Exit(0)
		default:
			fmt.Println("无效的选择，请重新输入。")
		}
	}
}

// 后台运行模式（适合系统服务）
func runDaemon() {
	logInfo("DNS 管理器已启动（后台模式）")
	logInfo("配置信息: Zone ID=%s, 记录名称=%s, 记录类型=%s", 
		config.ZoneID, config.RecordName, config.RecordType)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	// 立即执行一次
	checkAndUpdate()

	// 定时任务（每5秒检测一次）
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkAndUpdate()

		case sig := <-sigChan:
			switch sig {
			case syscall.SIGTERM, os.Interrupt:
				logInfo("收到停止信号，正在退出...")
				return
			case syscall.SIGHUP:
				logInfo("收到重载信号，重新加载配置...")
				reloadConfig()
			}

		case <-reloadChan:
			logInfo("重新加载配置...")
			reloadConfig()
		}
	}
}

// 重新加载配置
func reloadConfig() {
	newConfig := LoadConfig()
	if newConfig.APIToken == "" || newConfig.ZoneID == "" || newConfig.RecordName == "" {
		logError("新配置无效，保持使用旧配置")
		return
	}

	// 如果配置有变化，重新初始化客户端
	if newConfig.APIToken != config.APIToken {
		var err error
		cfClient, err = NewCloudflareClient(newConfig.APIToken)
		if err != nil {
			logError("重新初始化 Cloudflare 客户端失败: %v", err)
			return
		}
		logInfo("Cloudflare 客户端已重新初始化")
	}

	config = newConfig
	logInfo("配置已重新加载")
}

// 执行一次模式（适合 cron）
func runOnce() {
	logInfo("执行一次性 DNS 更新")
	checkAndUpdate()
	logInfo("更新完成")
}

func showMainMenu() {
	fmt.Println("\n========== 主菜单 ==========")
	fmt.Println("1. 开始监控 (每5秒自动检测并更新)")
	fmt.Println("2. 检查当前公网IP")
	fmt.Println("3. 立即更新DNS记录")
	fmt.Println("4. 查看DNS记录")
	fmt.Println("5. 配置设置")
	fmt.Println("6. 启动后台守护进程 (自动后台运行)")
	fmt.Println("7. 守护进程管理")
	fmt.Println("8. 退出")
	fmt.Println("===========================")
	fmt.Println("提示: 使用 --daemon 参数可直接后台运行")
	fmt.Println("提示: 使用 --manage 参数进入守护进程管理")
	fmt.Println("===========================")
}

func startMonitoring() {
	if running {
		fmt.Println("监控已在运行中...")
		return
	}

	fmt.Println("\n开始监控模式...")
	fmt.Printf("配置信息:\n")
	fmt.Printf("  Zone ID: %s\n", config.ZoneID)
	fmt.Printf("  记录名称: %s\n", config.RecordName)
	fmt.Printf("  记录类型: %s\n", config.RecordType)
	fmt.Printf("  检测间隔: 每5秒\n")
	fmt.Println("\n按 Ctrl+C 停止监控")
	fmt.Println("提示: 如需后台运行，请使用 --daemon 参数或配置为系统服务")
	fmt.Println()

	running = true

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 立即执行一次
	checkAndUpdate()

	// 定时任务（每5秒检测一次）
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkAndUpdate()
		case <-sigChan:
			fmt.Println("\n\n监控已停止")
			running = false
			return
		}
	}
}

func checkAndUpdate() {
	logInfo("正在检查公网IP...")

	// 获取IP（带服务信息）
	var ip string
	var serviceName string
	var err error
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ip, serviceName, err = ipChecker.GetPublicIPWithService()
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			logError("获取公网IP失败 (尝试 %d/%d): %v，1秒后重试...", i+1, maxRetries, err)
			time.Sleep(1 * time.Second)
		}
	}

	if err != nil {
		logError("获取公网IP失败: %v", err)
		return
	}

	logInfo("当前公网IP: %s (来源: %s)", ip, serviceName)

	// 如果IP没有变化，跳过更新
	if ip == currentIP {
		logInfo("IP未变化 (%s)，跳过更新", ip)
		return
	}

	// IP发生变化，需要确认（避免不同服务返回不同IP导致的误判）
	logInfo("检测到IP变化 (%s -> %s)，正在确认...", currentIP, ip)
	
	// 等待3秒后再次检测确认
	time.Sleep(3 * time.Second)
	
	// 再次获取IP进行确认
	confirmIP, confirmService, err := ipChecker.GetPublicIPWithService()
	if err != nil {
		logError("确认IP时失败: %v，取消更新", err)
		return
	}

	// 如果确认的IP与第一次检测的不同，说明可能是服务不稳定，取消更新
	if confirmIP != ip {
		logError("IP确认失败: 第一次检测到 %s，确认时检测到 %s (来源: %s)，可能是服务不稳定，取消更新", 
			ip, confirmIP, confirmService)
		return
	}

	// IP确认一致，检查当前DNS记录（支持多机器场景）
	logInfo("IP变化已确认 (%s -> %s)，正在检查DNS记录...", currentIP, ip)
	
	// 获取所有匹配的DNS记录
	allRecords, err := cfClient.GetAllDNSRecords(config.ZoneID, config.RecordName, config.RecordType)
	if err != nil {
		logInfo("未找到现有DNS记录，将创建新记录")
	} else {
		logInfo("找到 %d 个DNS记录", len(allRecords))
		// 检查是否已有指向本机IP的记录
		hasCurrentIP := false
		for _, record := range allRecords {
			if record.Content == ip {
				hasCurrentIP = true
				logInfo("已存在指向本机IP (%s) 的DNS记录，无需更新", ip)
				currentIP = ip
				return
			}
		}
		if !hasCurrentIP {
			logInfo("未找到指向本机IP的记录，将创建或更新记录")
		}
	}
	
	// 使用更新或创建逻辑（支持多机器：每个机器维护自己的A记录）
	logInfo("正在更新或创建DNS记录: %s -> %s", config.RecordName, ip)
	
	// 获取默认TTL（如果记录存在，使用现有记录的TTL；否则使用3600）
	defaultTTL := 3600
	if len(allRecords) > 0 {
		defaultTTL = allRecords[0].TTL
	}
	
	updateSuccess := false
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		// 使用更新或创建逻辑（支持多机器：每个机器维护自己的A记录）
		// 如果存在指向旧IP的记录，会更新它；否则创建新记录
		lastErr = cfClient.UpdateOrCreateDNSRecord(config.ZoneID, config.RecordName, config.RecordType, ip, defaultTTL, currentIP)
		if lastErr == nil {
			updateSuccess = true
			break
		}
		
		if i < maxRetries-1 {
			logError("DNS更新/创建失败 (尝试 %d/%d): %v，2秒后重试...", i+1, maxRetries, lastErr)
			time.Sleep(2 * time.Second)
		}
	}

	if !updateSuccess {
		logError("DNS更新/创建失败: %v", lastErr)
		return
	}

	// 验证记录是否存在
	verifyRecords, err := cfClient.GetAllDNSRecords(config.ZoneID, config.RecordName, config.RecordType)
	if err != nil {
		logError("验证DNS记录失败: %v，但更新可能已成功", err)
	} else {
		// 检查是否包含本机IP
		found := false
		for _, record := range verifyRecords {
			if record.Content == ip {
				found = true
				break
			}
		}
		if found {
			logInfo("DNS记录验证成功: %s 现在包含IP %s (共 %d 个A记录)", config.RecordName, ip, len(verifyRecords))
		} else {
			logError("DNS记录验证失败: 未找到指向 %s 的记录", ip)
		}
	}

	logInfo("DNS记录已成功更新/创建: %s -> %s", config.RecordName, ip)
	currentIP = ip
}

func checkCurrentIP() {
	fmt.Println("\n正在检查当前公网IP...")
	ip, service, err := ipChecker.GetPublicIPWithService()
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
		return
	}
	fmt.Printf("当前公网IP: %s (来源: %s)\n", ip, service)
}

func updateDNSNow() {
	fmt.Println("\n正在获取当前公网IP...")
	ip, service, err := ipChecker.GetPublicIPWithService()
	if err != nil {
		fmt.Printf("❌ 获取公网IP失败: %v\n", err)
		return
	}

	fmt.Printf("当前公网IP: %s (来源: %s)\n", ip, service)
	fmt.Printf("正在更新DNS记录 %s...\n", config.RecordName)

	err = cfClient.UpdateDNSRecord(config.ZoneID, config.RecordName, config.RecordType, ip)
	if err != nil {
		fmt.Printf("❌ 更新失败: %v\n", err)
		return
	}

	fmt.Printf("✓ DNS记录已成功更新: %s -> %s\n", config.RecordName, ip)
	currentIP = ip
}

func viewDNSRecords() {
	fmt.Println("\n正在获取DNS记录...")
	records, err := cfClient.ListDNSRecords(config.ZoneID, config.RecordName)
	if err != nil {
		fmt.Printf("❌ 获取失败: %v\n", err)
		return
	}

	if len(records) == 0 {
		fmt.Println("未找到匹配的DNS记录")
		return
	}

	fmt.Println("\nDNS记录列表:")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-30s %-10s %-20s %-10s\n", "名称", "类型", "内容", "TTL")
	fmt.Println(strings.Repeat("-", 80))
	for _, record := range records {
		fmt.Printf("%-30s %-10s %-20s %-10d\n", 
			record.Name, record.Type, record.Content, record.TTL)
	}
	fmt.Println(strings.Repeat("-", 80))
}

func interactiveConfig() {
	fmt.Println("\n========== 配置向导 ==========")
	
	// API Token
	fmt.Println("\n1. Cloudflare API Token")
	fmt.Println("   请在 Cloudflare 控制台创建 API Token")
	fmt.Println("   权限: Zone - DNS - Edit")
	fmt.Println("   访问: 选择你的域名")
	
	token := getUserInput("请输入 API Token: ")
	if token == "" {
		fmt.Println("API Token 不能为空")
		return
	}

	// Zone ID
	fmt.Println("\n2. Zone ID")
	fmt.Println("   在 Cloudflare 域名概览页面右侧可以找到 Zone ID")
	zoneID := getUserInput("请输入 Zone ID: ")
	if zoneID == "" {
		fmt.Println("Zone ID 不能为空")
		return
	}

	// 记录名称
	fmt.Println("\n3. DNS 记录名称")
	fmt.Println("   例如: subdomain.example.com 或 @ (表示根域名)")
	recordName := getUserInput("请输入记录名称: ")
	if recordName == "" {
		fmt.Println("记录名称不能为空")
		return
	}

	// 记录类型
	fmt.Println("\n4. DNS 记录类型")
	fmt.Println("   通常为 A (IPv4) 或 AAAA (IPv6)")
	recordType := getUserInput("请输入记录类型 (默认: A): ")
	if recordType == "" {
		recordType = "A"
	}

	// 保存配置
	config = &Config{
		APIToken:   token,
		ZoneID:     zoneID,
		RecordName: recordName,
		RecordType: recordType,
	}

	if err := SaveConfig(config); err != nil {
		fmt.Printf("❌ 保存配置失败: %v\n", err)
		return
	}

	fmt.Println("\n✓ 配置已保存！")
}

func startBackgroundDaemon() {
	// 检查是否有守护进程在运行
	processes, err := listDaemonProcesses()
	hasExistingDaemon := false
	if err == nil && len(processes) > 0 {
		// 过滤出守护进程（包含 --daemon 参数）
		daemonProcesses := []ProcessInfo{}
		for _, proc := range processes {
			if strings.Contains(proc.Command, "--daemon") && proc.PID != os.Getpid() {
				daemonProcesses = append(daemonProcesses, proc)
			}
		}

		if len(daemonProcesses) > 0 {
			hasExistingDaemon = true
			fmt.Printf("\n检测到已有 %d 个守护进程在运行:\n", len(daemonProcesses))
			for _, proc := range daemonProcesses {
				fmt.Printf("  - PID: %d\n", proc.PID)
			}
			fmt.Println("\n正在停止所有现有守护进程...")

			// 停止所有守护进程
			if err := stopAllDaemonProcesses(); err != nil {
				fmt.Printf("❌ 停止现有进程时出错: %v\n", err)
				fmt.Println("是否继续？(y/N)")
				confirm := getUserInput("")
				if confirm != "y" && confirm != "Y" {
					fmt.Println("已取消")
					return
				}
			} else {
				fmt.Println("✓ 所有现有守护进程已停止")
				// 等待一下确保进程完全退出
				time.Sleep(1 * time.Second)
			}
		}
	}

	// 再次检查PID文件（可能还有残留）
	pid, err := getPID()
	if err == nil {
		if isProcessRunning(pid) {
			fmt.Printf("\n检测到残留的PID文件（PID: %d），正在清理...\n", pid)
			removePIDFile()
			// 如果进程还在，尝试停止
			if isProcessRunning(pid) {
				process, err := os.FindProcess(pid)
				if err == nil {
					process.Signal(syscall.SIGTERM)
					time.Sleep(500 * time.Millisecond)
					if isProcessRunning(pid) {
						process.Signal(syscall.SIGKILL)
						time.Sleep(500 * time.Millisecond)
					}
				}
			}
			fmt.Println("✓ 已清理残留的PID文件")
		} else {
			// 进程不存在，清理PID文件
			removePIDFile()
		}
	}

	// 如果检测到已有服务，先删除配置，然后重新配置
	if hasExistingDaemon {
		fmt.Println("\n========== 清理配置 ==========")
		fmt.Println("检测到已有服务，正在删除所有相关配置...")
		
		// 删除配置文件
		if err := DeleteConfig(); err != nil {
			fmt.Printf("⚠️  删除配置文件时出错: %v\n", err)
		} else {
			fmt.Println("✓ 配置文件已删除")
		}

		// 清空内存中的配置
		config = &Config{
			RecordType: "A",
		}
		cfClient = nil

		fmt.Println("\n========== 重新配置 ==========")
		fmt.Println("所有配置已清除，请按照提示重新输入以下信息：")
		fmt.Println()

		// 进行交互式配置
		interactiveConfig()

		// 重新加载配置
		config = LoadConfig()
		if config.APIToken == "" || config.ZoneID == "" || config.RecordName == "" {
			fmt.Println("❌ 配置未完成，无法启动守护进程")
			return
		}

		// 重新初始化客户端
		var clientErr error
		cfClient, clientErr = NewCloudflareClient(config.APIToken)
		if clientErr != nil {
			fmt.Printf("❌ 初始化 Cloudflare 客户端失败: %v\n", clientErr)
			fmt.Println("请检查 API Token 是否正确")
			return
		}

		fmt.Println("\n✓ 配置完成！")
		fmt.Println()
	} else {
		// 检查配置是否存在（首次运行）
		if config.APIToken == "" || config.ZoneID == "" || config.RecordName == "" {
			fmt.Println("\n========== 首次配置 ==========")
			fmt.Println("检测到未配置，需要先进行配置才能启动守护进程")
			fmt.Println("请按照提示输入以下信息：")
			fmt.Println()

			// 进行交互式配置
			interactiveConfig()

			// 重新加载配置
			config = LoadConfig()
			if config.APIToken == "" || config.ZoneID == "" || config.RecordName == "" {
				fmt.Println("❌ 配置未完成，无法启动守护进程")
				return
			}

			// 重新初始化客户端
			var clientErr error
			cfClient, clientErr = NewCloudflareClient(config.APIToken)
			if clientErr != nil {
				fmt.Printf("❌ 初始化 Cloudflare 客户端失败: %v\n", clientErr)
				fmt.Println("请检查 API Token 是否正确")
				return
			}

			fmt.Println("\n✓ 配置完成！")
			fmt.Println()
		}
	}

	// 验证配置有效性
	fmt.Println("正在验证配置...")
	if err := verifyConfig(); err != nil {
		fmt.Printf("❌ 配置验证失败: %v\n", err)
		fmt.Println("请检查配置是否正确，或使用菜单选项 5 重新配置")
		return
	}
	fmt.Println("✓ 配置验证通过")

	fmt.Println("\n正在启动后台守护进程...")
	fmt.Println("程序将在后台自动运行，每5秒检测一次IP变化")
	fmt.Printf("配置信息:\n")
	fmt.Printf("  Zone ID: %s\n", config.ZoneID)
	fmt.Printf("  记录名称: %s\n", config.RecordName)
	fmt.Printf("  记录类型: %s\n", config.RecordType)
	fmt.Println()
	
	// 获取可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ 获取可执行文件路径失败: %v\n", err)
		return
	}

	// 使用绝对路径
	absPath, err := filepath.Abs(execPath)
	if err != nil {
		fmt.Printf("❌ 获取绝对路径失败: %v\n", err)
		return
	}

	// 启动守护进程
	cmd := exec.Command(absPath, "--daemon")
	cmd.Env = os.Environ()
	
	// 设置进程属性
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // 创建新的会话
	}

	// 重定向标准输入输出到 /dev/null
	nullFile, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err == nil {
		cmd.Stdin = nullFile
		cmd.Stdout = nullFile
		cmd.Stderr = nullFile
	}

	// 启动守护进程
	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ 启动守护进程失败: %v\n", err)
		return
	}

	fmt.Printf("✓ 守护进程已启动，PID: %d\n", cmd.Process.Pid)
	fmt.Println("程序已在后台运行，可以安全关闭终端")
	fmt.Println("使用 './dns_manager --status' 查看运行状态")
	fmt.Println("使用 './dns_manager --stop' 停止守护进程")
	fmt.Println("使用 './dns_manager --info' 查看详细信息")
	
	// 保存PID到文件
	savePID(cmd.Process.Pid)
}

// verifyConfig 验证配置有效性
func verifyConfig() error {
	// 验证 API Token
	if config.APIToken == "" {
		return fmt.Errorf("API Token 不能为空")
	}

	// 验证 Zone ID
	if config.ZoneID == "" {
		return fmt.Errorf("Zone ID 不能为空")
	}

	// 验证记录名称
	if config.RecordName == "" {
		return fmt.Errorf("记录名称不能为空")
	}

	// 验证记录类型
	if config.RecordType != "A" && config.RecordType != "AAAA" {
		return fmt.Errorf("记录类型必须是 A 或 AAAA")
	}

	// 尝试连接 Cloudflare API 验证配置
	if cfClient == nil {
		var err error
		cfClient, err = NewCloudflareClient(config.APIToken)
		if err != nil {
			return fmt.Errorf("初始化 Cloudflare 客户端失败: %v", err)
		}
	}

	// 尝试获取DNS记录验证配置
	_, err := cfClient.ListDNSRecords(config.ZoneID, config.RecordName)
	if err != nil {
		return fmt.Errorf("无法访问 Cloudflare API 或配置错误: %v", err)
	}

	return nil
}

// manageDaemonMenu 守护进程管理菜单
func manageDaemonMenu() {
	for {
		fmt.Println("\n========== 守护进程管理 ==========")
		fmt.Println("1. 查看守护进程状态")
		fmt.Println("2. 查看详细信息")
		fmt.Println("3. 列出所有进程")
		fmt.Println("4. 停止守护进程")
		fmt.Println("5. 强制终止守护进程")
		fmt.Println("6. 清理无效PID文件")
		fmt.Println("7. 返回主菜单")
		fmt.Println("================================")

		choice := getUserInput("请选择操作 (1-7): ")

		switch choice {
		case "1":
			pid, err := getPID()
			if err != nil {
				fmt.Println("守护进程未运行（未找到PID文件）")
			} else if isProcessRunning(pid) {
				fmt.Printf("✓ 守护进程正在运行，PID: %d\n", pid)
			} else {
				fmt.Printf("✗ 守护进程未运行（PID文件存在但进程不存在，PID: %d）\n", pid)
				fmt.Println("提示: 选择选项 6 清理无效的PID文件")
			}

		case "2":
			info, err := getDaemonInfo()
			if err != nil {
				fmt.Printf("❌ 获取信息失败: %v\n", err)
			} else {
				printDaemonInfo(info)
			}

		case "3":
			processes, err := listDaemonProcesses()
			if err != nil {
				fmt.Printf("❌ 列出进程失败: %v\n", err)
			} else if len(processes) == 0 {
				fmt.Println("未找到运行中的 dns_manager 进程")
			} else {
				fmt.Printf("\n找到 %d 个 dns_manager 进程:\n", len(processes))
				fmt.Println(strings.Repeat("-", 80))
				for _, proc := range processes {
					fmt.Printf("PID: %d\n%s\n", proc.PID, proc.Command)
					fmt.Println(strings.Repeat("-", 80))
				}
			}

		case "4":
			if err := stopDaemon(); err != nil {
				fmt.Printf("❌ 停止失败: %v\n", err)
			} else {
				fmt.Println("✓ 守护进程已停止")
			}

		case "5":
			fmt.Println("警告: 强制终止可能导致数据丢失，是否继续？(y/N)")
			confirm := getUserInput("")
			if confirm == "y" || confirm == "Y" {
				if err := killDaemon(); err != nil {
					fmt.Printf("❌ 强制终止失败: %v\n", err)
				} else {
					fmt.Println("✓ 守护进程已强制终止")
				}
			} else {
				fmt.Println("已取消")
			}

		case "6":
			if err := cleanupPIDFile(); err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("✓ PID文件检查完成，无需清理")
			}

		case "7":
			return

		default:
			fmt.Println("无效的选择，请重新输入。")
		}
	}
}

// printDaemonInfo 打印守护进程详细信息
func printDaemonInfo(info map[string]interface{}) {
	fmt.Println("\n========== 守护进程信息 ==========")
	
	if running, ok := info["running"].(bool); ok {
		if running {
			fmt.Println("状态: ✓ 正在运行")
		} else {
			fmt.Println("状态: ✗ 未运行")
		}
	}

	if pid, ok := info["pid"].(int); ok {
		fmt.Printf("PID: %d\n", pid)
	}

	if pidFile, ok := info["pid_file"].(string); ok {
		fmt.Printf("PID文件: %s\n", pidFile)
	}

	if logFile, ok := info["log_file"].(string); ok {
		fmt.Printf("日志文件: %s\n", logFile)
		if logSize, ok := info["log_size"].(int64); ok {
			fmt.Printf("日志大小: %d 字节 (%.2f KB)\n", logSize, float64(logSize)/1024)
		}
	}

	if details, ok := info["details"].(string); ok && details != "" {
		fmt.Println("\n进程详情:")
		fmt.Println(details)
	}

	if err, ok := info["error"].(string); ok {
		fmt.Printf("错误: %s\n", err)
	}

	fmt.Println("==================================")
}

func getUserInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}
