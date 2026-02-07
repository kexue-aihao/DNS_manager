package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// daemonize 将当前进程转换为守护进程
func daemonize() error {
	// 检查是否已经是守护进程
	if os.Getppid() == 1 {
		// 已经是守护进程，直接返回
		return nil
	}

	// 获取可执行文件路径
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %v", err)
	}

	// 使用绝对路径
	absPath, err := filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %v", err)
	}

	// 重新启动自己，作为守护进程
	cmd := exec.Command(absPath, os.Args[1:]...)
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
		return fmt.Errorf("启动守护进程失败: %v", err)
	}

	fmt.Printf("守护进程已启动，PID: %d\n", cmd.Process.Pid)
	fmt.Println("程序已在后台运行，可以安全关闭终端")
	
	// 保存PID到文件
	savePID(cmd.Process.Pid)

	// 退出父进程
	os.Exit(0)
	return nil
}

// savePID 保存进程ID到文件
func savePID(pid int) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	pidDir := filepath.Join(homeDir, ".go_dns_manager")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return err
	}

	pidFile := filepath.Join(pidDir, "dns_manager.pid")
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// getPID 从文件读取进程ID
func getPID() (int, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	pidFile := filepath.Join(homeDir, ".go_dns_manager", "dns_manager.pid")
	
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, err
	}

	return pid, nil
}

// isProcessRunning 检查进程是否在运行
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	
	// 发送信号0来检查进程是否存在
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// stopDaemon 停止守护进程
func stopDaemon() error {
	pid, err := getPID()
	if err != nil {
		return fmt.Errorf("无法读取PID文件: %v", err)
	}

	if !isProcessRunning(pid) {
		// 进程不存在，清理PID文件
		removePIDFile()
		return fmt.Errorf("进程 %d 未运行（已清理PID文件）", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("无法找到进程: %v", err)
	}

	// 发送终止信号
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("无法终止进程: %v", err)
	}

	// 等待进程退出（最多等待5秒）
	for i := 0; i < 50; i++ {
		if !isProcessRunning(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 如果进程还在运行，强制终止
	if isProcessRunning(pid) {
		process.Signal(syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond)
	}

	// 删除PID文件
	removePIDFile()

	fmt.Printf("守护进程 (PID: %d) 已停止\n", pid)
	return nil
}

// killDaemon 强制删除守护进程
func killDaemon() error {
	pid, err := getPID()
	if err != nil {
		return fmt.Errorf("无法读取PID文件: %v", err)
	}

	if !isProcessRunning(pid) {
		removePIDFile()
		return fmt.Errorf("进程 %d 未运行（已清理PID文件）", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("无法找到进程: %v", err)
	}

	// 强制终止
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("无法强制终止进程: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
	removePIDFile()

	fmt.Printf("守护进程 (PID: %d) 已强制终止\n", pid)
	return nil
}

// removePIDFile 删除PID文件
func removePIDFile() {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "."
	}
	pidFile := filepath.Join(homeDir, ".go_dns_manager", "dns_manager.pid")
	os.Remove(pidFile)
}

// listDaemonProcesses 列出所有dns_manager进程
func listDaemonProcesses() ([]ProcessInfo, error) {
	var processes []ProcessInfo

	// 使用ps命令查找所有dns_manager进程
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行ps命令失败: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "dns_manager") && !strings.Contains(line, "grep") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				pid, err := strconv.Atoi(fields[1])
				if err == nil {
					processes = append(processes, ProcessInfo{
						PID:     pid,
						Command: line,
					})
				}
			}
		}
	}

	return processes, nil
}

// ProcessInfo 进程信息
type ProcessInfo struct {
	PID     int
	Command string
}

// getDaemonInfo 获取守护进程详细信息
func getDaemonInfo() (map[string]interface{}, error) {
	info := make(map[string]interface{})

	// 获取PID
	pid, err := getPID()
	if err != nil {
		info["running"] = false
		info["error"] = "未找到PID文件"
		return info, nil
	}

	info["pid"] = pid
	info["running"] = isProcessRunning(pid)

	if !isProcessRunning(pid) {
		info["error"] = "进程不存在"
		return info, nil
	}

	// 获取进程详细信息
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid,ppid,user,start,time,cmd")
	output, err := cmd.Output()
	if err == nil {
		info["details"] = strings.TrimSpace(string(output))
	}

	// 获取PID文件路径
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir = "."
	}
	pidFile := filepath.Join(homeDir, ".go_dns_manager", "dns_manager.pid")
	info["pid_file"] = pidFile

	// 检查日志文件
	logDir := filepath.Join(homeDir, ".go_dns_manager", "logs")
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(logDir, fmt.Sprintf("dns_manager_%s.log", today))
	if _, err := os.Stat(logFile); err == nil {
		info["log_file"] = logFile
		// 获取日志文件大小
		if stat, err := os.Stat(logFile); err == nil {
			info["log_size"] = stat.Size()
		}
	}

	return info, nil
}

// cleanupPIDFile 清理无效的PID文件
func cleanupPIDFile() error {
	pid, err := getPID()
	if err != nil {
		return nil // 没有PID文件，无需清理
	}

	if !isProcessRunning(pid) {
		removePIDFile()
		return fmt.Errorf("已清理无效的PID文件（进程 %d 不存在）", pid)
	}

	return nil
}

// stopAllDaemonProcesses 停止所有 dns_manager 守护进程
func stopAllDaemonProcesses() error {
	processes, err := listDaemonProcesses()
	if err != nil {
		return fmt.Errorf("列出进程失败: %v", err)
	}

	if len(processes) == 0 {
		return nil // 没有进程在运行
	}

	fmt.Printf("检测到 %d 个运行中的 dns_manager 进程，正在停止...\n", len(processes))

	var stoppedCount int
	var failedCount int

	for _, proc := range processes {
		// 跳过当前进程（如果是通过命令行调用的）
		if proc.PID == os.Getpid() {
			continue
		}

		process, err := os.FindProcess(proc.PID)
		if err != nil {
			fmt.Printf("  警告: 无法找到进程 %d: %v\n", proc.PID, err)
			failedCount++
			continue
		}

		// 先尝试优雅停止
		if err := process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("  警告: 无法发送 SIGTERM 到进程 %d: %v\n", proc.PID, err)
			// 如果 SIGTERM 失败，尝试强制终止
			if err := process.Signal(syscall.SIGKILL); err != nil {
				fmt.Printf("  错误: 无法终止进程 %d: %v\n", proc.PID, err)
				failedCount++
				continue
			}
		}

		// 等待进程退出（最多等待2秒）
		stopped := false
		for i := 0; i < 20; i++ {
			if !isProcessRunning(proc.PID) {
				stopped = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// 如果进程还在运行，强制终止
		if !stopped && isProcessRunning(proc.PID) {
			if err := process.Signal(syscall.SIGKILL); err != nil {
				fmt.Printf("  错误: 无法强制终止进程 %d: %v\n", proc.PID, err)
				failedCount++
				continue
			}
			time.Sleep(500 * time.Millisecond)
		}

		if !isProcessRunning(proc.PID) {
			fmt.Printf("  ✓ 已停止进程 PID: %d\n", proc.PID)
			stoppedCount++
		} else {
			fmt.Printf("  ✗ 无法停止进程 PID: %d\n", proc.PID)
			failedCount++
		}
	}

	// 清理PID文件
	removePIDFile()

	// 清理可能残留的进程（通过进程名查找）
	cleanupRemainingProcesses()

	if failedCount > 0 {
		return fmt.Errorf("成功停止 %d 个进程，%d 个进程停止失败", stoppedCount, failedCount)
	}

	if stoppedCount > 0 {
		fmt.Printf("✓ 已成功停止 %d 个进程\n", stoppedCount)
	}

	return nil
}

// cleanupRemainingProcesses 清理残留的进程
func cleanupRemainingProcesses() {
	// 再次检查是否还有进程
	processes, err := listDaemonProcesses()
	if err != nil {
		return
	}

	for _, proc := range processes {
		// 跳过当前进程
		if proc.PID == os.Getpid() {
			continue
		}

		// 只处理守护进程（包含 --daemon 参数）
		if !strings.Contains(proc.Command, "--daemon") {
			continue
		}

		process, err := os.FindProcess(proc.PID)
		if err != nil {
			continue
		}

		// 强制终止
		process.Signal(syscall.SIGKILL)
		time.Sleep(200 * time.Millisecond)
	}
}
