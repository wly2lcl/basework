package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// logLevelNames 日志级别名称映射
var logLevelNames = map[LogLevel]string{
	LogLevelDebug: "debug",
	LogLevelInfo:  "info",
	LogLevelWarn:  "warn",
	LogLevelError: "error",
}

// logLevelValues 日志级别名称到值的映射
var logLevelValues = map[string]LogLevel{
	"debug": LogLevelDebug,
	"info":  LogLevelInfo,
	"warn":  LogLevelWarn,
	"error": LogLevelError,
}

// ParseLogLevel 将字符串解析为日志级别，未知级别默认为 info
func ParseLogLevel(s string) LogLevel {
	if l, ok := logLevelValues[s]; ok {
		return l
	}
	return LogLevelInfo
}

// logEntry 日志条目结构，序列化为 JSON
type logEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Logger 是结构化日志记录器，支持日志级别和 JSON 格式输出
type Logger struct {
	mu     sync.Mutex
	level  LogLevel
	output io.Writer
}

// NewLogger 创建日志记录器。
// level 为日志级别，output 为输出目标（nil 时默认 stdout）。
func NewLogger(level LogLevel, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}
	return &Logger{
		level:  level,
		output: output,
	}
}

// SetLevel 设置日志级别
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput 设置输出目标
func (l *Logger) SetOutput(output io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = output
}

// Log 记录指定级别的日志，支持附加字段
func (l *Logger) Log(level LogLevel, msg string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     logLevelNames[level],
		Message:   msg,
		Fields:    fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.output, `{"level":"error","msg":"failed to marshal log entry","error":%q}%c`, err.Error(), '\n')
		return
	}

	data = append(data, '\n')
	l.output.Write(data)
}

// Debug 记录 debug 级别日志
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.Log(LogLevelDebug, msg, mergeFields(fields...))
}

// Info 记录 info 级别日志
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.Log(LogLevelInfo, msg, mergeFields(fields...))
}

// Warn 记录 warn 级别日志
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.Log(LogLevelWarn, msg, mergeFields(fields...))
}

// Error 记录 error 级别日志
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	l.Log(LogLevelError, msg, mergeFields(fields...))
}

// mergeFields 合并多个 field map，后面的覆盖前面的
func mergeFields(fields ...map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 {
		return fields[0]
	}
	merged := make(map[string]interface{})
	for _, f := range fields {
		for k, v := range f {
			merged[k] = v
		}
	}
	return merged
}
