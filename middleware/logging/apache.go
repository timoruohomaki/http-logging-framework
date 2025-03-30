package logging

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogFormat defines the logging format type
type LogFormat string

const (
	// CommonLogFormat is the standard Apache Common Log Format
	// %h %l %u %t \"%r\" %>s %b
	CommonLogFormat LogFormat = "common"

	// CombinedLogFormat is the Apache Combined Log Format (Common + Referer + User-Agent)
	// %h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\"
	CombinedLogFormat LogFormat = "combined"
)

// ApacheLogConfig holds configuration for Apache-style access logging
type ApacheLogConfig struct {
	LogPath    string
	MaxSize    int // megabytes
	MaxBackups int // number of backups
	MaxAge     int // days
	Compress   bool
	Format     LogFormat
}

// DefaultApacheLogConfig returns a default configuration
func DefaultApacheLogConfig() ApacheLogConfig {
	return ApacheLogConfig{
		LogPath:    "/var/log/apache2/access.log",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
		Format:     CommonLogFormat,
	}
}

// responseWrapper is a custom ResponseWriter that captures status code and bytes written
type responseWrapper struct {
	http.ResponseWriter
	status int
	size   int
}

// WriteHeader captures the status code
func (rw *responseWrapper) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// Write captures the size of the response
func (rw *responseWrapper) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// secureLogFile ensures the log file exists with secure permissions
func secureLogFile(logPath string) error {
	// Create directory if it doesn't exist
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Check if file exists
	fileInfo, err := os.Stat(logPath)
	if os.IsNotExist(err) {
		// Create the file with secure permissions (0640 = owner rw, group r, others none)
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
		if err != nil {
			return fmt.Errorf("failed to create log file: %w", err)
		}
		file.Close()
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check log file: %w", err)
	}

	// If file exists, check its permissions
	if fileInfo.Mode().Perm() != 0640 {
		// Fix permissions if they're not correct
		if err := os.Chmod(logPath, 0640); err != nil {
			return fmt.Errorf("failed to set log file permissions: %w", err)
		}
	}

	return nil
}

// SecureRotatedLogs is a function that should be called periodically to secure rotated log files
func SecureRotatedLogs(logPath string) error {
	// Get the base filename without extension
	dir, file := filepath.Split(logPath)
	ext := filepath.Ext(file)
	base := file[:len(file)-len(ext)]

	// Find all log files matching the pattern (including rotated ones)
	pattern := filepath.Join(dir, base+"*"+ext+"*") // Matches original and rotated logs
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find log files: %w", err)
	}

	// Set proper permissions on all matching files
	for _, match := range matches {
		fileInfo, err := os.Stat(match)
		if err != nil {
			return fmt.Errorf("failed to stat log file %s: %w", match, err)
		}

		if fileInfo.Mode().Perm() != 0640 {
			if err := os.Chmod(match, 0640); err != nil {
				return fmt.Errorf("failed to chmod log file %s: %w", match, err)
			}
		}
	}

	return nil
}

// NewApacheLogger creates a Zap logger configured for Apache Log Formats
func NewApacheLogger(config ApacheLogConfig) (*zap.Logger, error) {
	// Secure the log file before configuring lumberjack
	if err := secureLogFile(config.LogPath); err != nil {
		return nil, err
	}

	// Configure lumberjack for log rotation
	logWriter := &lumberjack.Logger{
		Filename:   config.LogPath,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
	}

	// Create custom encoder config to avoid timestamps in the log entry
	// since we're already formatting in Apache format
	encoderConfig := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "", // Omit level
		TimeKey:        "", // Omit timestamp
		NameKey:        "logger",
		CallerKey:      "", // Omit caller
		FunctionKey:    zapcore.OmitKey,
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create custom encoder and core
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(logWriter), zapcore.InfoLevel)

	// Create logger with custom core
	return zap.New(core), nil
}

// formatLogEntry formats a log entry according to the specified format
func formatLogEntry(r *http.Request, wrapper *responseWrapper, start time.Time, format LogFormat) string {
	// Get the remote address
	remoteAddr := r.RemoteAddr

	// Format the time in Apache log format: [day/month/year:hour:minute:second zone]
	timeFormatted := start.Format("[02/Jan/2006:15:04:05 -0700]")

	// Base log entry in Common Log Format
	// %h %l %u %t \"%r\" %>s %b
	logEntry := fmt.Sprintf("%s - - %s \"%s %s %s\" %d %d",
		remoteAddr,
		timeFormatted,
		r.Method,
		r.RequestURI,
		r.Proto,
		wrapper.status,
		wrapper.size,
	)

	// If Combined Log Format is requested, add Referer and User-Agent
	if format == CombinedLogFormat {
		referer := r.Header.Get("Referer")
		if referer == "" {
			referer = "-"
		}

		userAgent := r.Header.Get("User-Agent")
		if userAgent == "" {
			userAgent = "-"
		}

		// Add Referer and User-Agent to the log entry
		logEntry = fmt.Sprintf("%s \"%s\" \"%s\"",
			logEntry,
			referer,
			userAgent,
		)
	}

	return logEntry
}

// ApacheLogMiddleware creates middleware that logs requests in the configured Apache Log Format
func ApacheLogMiddleware(logger *zap.Logger, format LogFormat) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response wrapper to capture the status code and bytes written
			wrapper := &responseWrapper{
				ResponseWriter: w,
				status:         200, // Default status is 200
				size:           0,
			}

			// Process the request
			next.ServeHTTP(wrapper, r)

			// Format the log entry according to the specified format
			logEntry := formatLogEntry(r, wrapper, start, format)

			// Log using zap
			logger.Info(logEntry)
		})
	}
}
