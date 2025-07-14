// utils/logger.go

package utils

import (
    "fmt"
    "io"
    "log"
    "os"
)

var (
    InfoLogger    *log.Logger
    WarningLogger *log.Logger
    ErrorLogger   *log.Logger
)

// LogLevel 控制日志输出级别
type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelWarn
    LogLevelError
)

var currentLogLevel = LogLevelInfo // 默认日志级别

func SetLogLevel(level LogLevel) {
    currentLogLevel = level
}

func init() {
    log.SetFlags(0) // 避免默认日志前缀干扰
}

// InitLogger 初始化日志系统（文件 + 控制台）
func InitLogger(logFilePath string) error {
    // 创建日志目录
    logDir := "/var/log/portknock"
    if err := os.MkdirAll(logDir, 0755); err != nil {
        return fmt.Errorf("无法创建日志目录: %v", err)
    }

    // 打开日志文件
    file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return fmt.Errorf("无法打开日志文件: %v", err)
    }

    // 多写入器：同时输出到控制台和文件
    multiWriter := io.MultiWriter(os.Stdout, file)

    // 设置日志前缀
    InfoLogger = log.New(multiWriter, "[INFO] ", log.LstdFlags|log.Lshortfile)
    WarningLogger = log.New(multiWriter, "[WARN] ", log.LstdFlags|log.Lshortfile)
    ErrorLogger = log.New(multiWriter, "[ERROR] ", log.LstdFlags|log.Lshortfile)

    return nil
}

// LogInfo 输出 INFO 日志
func LogInfo(format string, v ...interface{}) {
    if currentLogLevel <= LogLevelInfo {
        InfoLogger.Output(2, fmt.Sprintf(format, v...))
    }
}

// LogWarn 输出 WARN 日志
func LogWarn(format string, v ...interface{}) {
    if currentLogLevel <= LogLevelWarn {
        WarningLogger.Output(2, fmt.Sprintf(format, v...))
    }
}

// LogError 输出 ERROR 日志
func LogError(format string, v ...interface{}) {
    if currentLogLevel <= LogLevelError {
        ErrorLogger.Output(2, fmt.Sprintf(format, v...))
    }
}

// LogDebug 输出 DEBUG 日志
func LogDebug(format string, v ...interface{}) {
    if currentLogLevel <= LogLevelDebug {
        InfoLogger.Output(2, fmt.Sprintf("[DEBUG] "+format, v...))
    }
}