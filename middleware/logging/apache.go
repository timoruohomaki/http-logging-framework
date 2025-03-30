package logging

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// ApacheLogConfig holds configuration for Apache-style access logging
type ApacheLogConfig struct {
	LogPath    string
	MaxSize    int // megabytes
	MaxBackups int // number of backups
	MaxAge     int // days
	Compress   bool
}

// DefaultApacheLogConfig returns a default configuration
// NOTE: Default path is for Linux and macOS
func DefaultApacheLogConfig() ApacheLogConfig {
	return ApacheLogConfig{
		LogPath:    "/var/log/apache2/access.log",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
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

// NewApacheLogger creates a Zap logger configured for Apache Common Log Format
func NewApacheLogger(config ApacheLogConfig) (*zap.Logger, error) {
	// Configure lumberjack for log rotation
	logWriter := &lumberjack.Logger{
		Filename:   config.LogPath,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
		Compress:   config.Compress,
	}

	// Create custom encoder config to avoid timestamps in the log entry
	// since we're already formatting in Apache CLF
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

// ApacheCommonLogMiddleware creates middleware that logs requests in Apache Common Log Format
// TODO: Note that Common Log Format doesn't include referrer and user-agent
// They are supported in Combined Log Format that could be implemented as an alternative middleware

func ApacheCommonLogMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
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

			// Get the remote address
			remoteAddr := r.RemoteAddr

			// Format the time in Apache Common Log Format: [day/month/year:hour:minute:second zone]
			timeFormatted := start.Format("[02/Jan/2006:15:04:05 -0700]")

			// Create the log entry in Common Log Format
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

			// Log using zap
			logger.Info(logEntry)
		})
	}
}
