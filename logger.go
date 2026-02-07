package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Logger struct {
	fileLogger *log.Logger
	console    bool
	logFile    *os.File
}

var globalLogger *Logger

func initLogger(enableFileLog bool, enableConsole bool) error {
	globalLogger = &Logger{
		console: enableConsole,
	}

	if enableFileLog {
		// 创建日志目录
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		logDir := filepath.Join(homeDir, ".go_dns_manager", "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}

		// 创建日志文件（按日期命名）
		logFileName := fmt.Sprintf("dns_manager_%s.log", time.Now().Format("2006-01-02"))
		logPath := filepath.Join(logDir, logFileName)

		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %v", err)
		}

		globalLogger.logFile = file
		globalLogger.fileLogger = log.New(file, "", log.LstdFlags)
	}

	return nil
}

func (l *Logger) Info(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] %s", timestamp, message)

	if l.console {
		fmt.Println(logMessage)
	}

	if l.fileLogger != nil {
		l.fileLogger.Println(message)
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	message := fmt.Sprintf("ERROR: %s", fmt.Sprintf(format, v...))
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] %s", timestamp, message)

	if l.console {
		fmt.Fprintln(os.Stderr, logMessage)
	}

	if l.fileLogger != nil {
		l.fileLogger.Println(message)
	}
}

func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// 便捷函数
func logInfo(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Info(format, v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

func logError(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Error(format, v...)
	} else {
		fmt.Fprintf(os.Stderr, format+"\n", v...)
	}
}
