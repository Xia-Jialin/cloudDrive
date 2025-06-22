package logger

import (
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

// Config 日志配置
type Config struct {
	Level       string `yaml:"level" mapstructure:"level"`
	File        string `yaml:"file" mapstructure:"file"`
	MaxSize     int    `yaml:"max_size" mapstructure:"max_size"` // MB
	MaxAge      int    `yaml:"max_age" mapstructure:"max_age"`   // days
	MaxBackups  int    `yaml:"max_backups" mapstructure:"max_backups"`
	Compress    bool   `yaml:"compress" mapstructure:"compress"`
	Development bool   `yaml:"development" mapstructure:"development"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:       "info",
		File:        "logs/app.log",
		MaxSize:     100,
		MaxAge:      30,
		MaxBackups:  10,
		Compress:    true,
		Development: false,
	}
}

// InitLogger 初始化日志器
func InitLogger(config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	// 确保日志目录存在
	if config.File != "" {
		logDir := filepath.Dir(config.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}
	}

	// 设置日志级别
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(config.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	// 创建编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建核心组件
	var cores []zapcore.Core

	// 控制台输出
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	consoleCore := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		level,
	)
	cores = append(cores, consoleCore)

	// 文件输出
	if config.File != "" {
		fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
		writer := &lumberjack.Logger{
			Filename:   config.File,
			MaxSize:    config.MaxSize,
			MaxAge:     config.MaxAge,
			MaxBackups: config.MaxBackups,
			Compress:   config.Compress,
		}
		fileCore := zapcore.NewCore(
			fileEncoder,
			zapcore.AddSync(writer),
			level,
		)
		cores = append(cores, fileCore)
	}

	// 创建logger
	core := zapcore.NewTee(cores...)

	var options []zap.Option
	if config.Development {
		options = append(options, zap.Development())
	}
	options = append(options, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	Logger = zap.New(core, options...)
	Sugar = Logger.Sugar()

	return nil
}

// LogFields 结构化日志字段
type LogFields struct {
	RequestID  string        `json:"request_id,omitempty"`
	UserID     string        `json:"user_id,omitempty"`
	Method     string        `json:"method,omitempty"`
	Path       string        `json:"path,omitempty"`
	StatusCode int           `json:"status_code,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	ClientIP   string        `json:"client_ip,omitempty"`
	UserAgent  string        `json:"user_agent,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// ToZapFields 转换为zap字段
func (f *LogFields) ToZapFields() []zap.Field {
	var fields []zap.Field

	if f.RequestID != "" {
		fields = append(fields, zap.String("request_id", f.RequestID))
	}
	if f.UserID != "" {
		fields = append(fields, zap.String("user_id", f.UserID))
	}
	if f.Method != "" {
		fields = append(fields, zap.String("method", f.Method))
	}
	if f.Path != "" {
		fields = append(fields, zap.String("path", f.Path))
	}
	if f.StatusCode != 0 {
		fields = append(fields, zap.Int("status_code", f.StatusCode))
	}
	if f.Duration != 0 {
		fields = append(fields, zap.Duration("duration", f.Duration))
	}
	if f.ClientIP != "" {
		fields = append(fields, zap.String("client_ip", f.ClientIP))
	}
	if f.UserAgent != "" {
		fields = append(fields, zap.String("user_agent", f.UserAgent))
	}
	if f.Error != "" {
		fields = append(fields, zap.String("error", f.Error))
	}

	return fields
}

// Info 记录信息日志
func Info(msg string, fields *LogFields) {
	if Logger != nil {
		Logger.Info(msg, fields.ToZapFields()...)
	}
}

// Error 记录错误日志
func Error(msg string, fields *LogFields, err error) {
	if Logger != nil {
		zapFields := fields.ToZapFields()
		if err != nil {
			zapFields = append(zapFields, zap.Error(err))
		}
		Logger.Error(msg, zapFields...)
	}
}

// Warn 记录警告日志
func Warn(msg string, fields *LogFields) {
	if Logger != nil {
		Logger.Warn(msg, fields.ToZapFields()...)
	}
}

// Debug 记录调试日志
func Debug(msg string, fields *LogFields) {
	if Logger != nil {
		Logger.Debug(msg, fields.ToZapFields()...)
	}
}

// Sync 同步日志
func Sync() {
	if Logger != nil {
		Logger.Sync()
	}
}
