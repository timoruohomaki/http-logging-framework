package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"timoruohomaki/http-logging-framework/middleware/logging" // update according to your project name
)

func main() {
	// Create a development logger for server startup/shutdown messages
	serverLogger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("Failed to create server logger: %v\n", err)
		os.Exit(1)
	}
	defer serverLogger.Sync()

	// Create Apache logger with rotation
	config := logging.DefaultApacheLogConfig()

	// You can customize the config if needed
	// config.LogPath = "/custom/path/access.log"
	// config.MaxSize = 50

	// Set the log format - use Combined instead of Common if you want Referer and User-Agent
	config.Format = logging.CombinedLogFormat // or logging.CommonLogFormat

	accessLogger, err := logging.NewApacheLogger(config)
	if err != nil {
		serverLogger.Fatal("Failed to create access logger",
			zap.Error(err),
			zap.String("logPath", config.LogPath))
	}
	defer accessLogger.Sync()

	// Start a goroutine to periodically secure log files
	// This ensures rotated files also have correct permissions
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := logging.SecureRotatedLogs(config.LogPath); err != nil {
					serverLogger.Error("Failed to secure log files", zap.Error(err))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Define API routes
	mux := http.NewServeMux()

	// Example route
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Hello, World!"}`))
	})

	// Add the Apache Log Format middleware with the configured format
	handler := logging.ApacheLogMiddleware(accessLogger, config.Format)(mux)

	// Configure the HTTP server
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Run server in a goroutine so we can gracefully handle shutdown
	go func() {
		serverLogger.Info("Starting server",
			zap.String("address", ":8080"),
			zap.String("log_format", string(config.Format)))

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverLogger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	serverLogger.Info("Shutting down server...")

	// Stop the log security goroutine
	cancel()

	// Create a deadline for server shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		serverLogger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	// One final check to secure log files before exiting
	if err := logging.SecureRotatedLogs(config.LogPath); err != nil {
		serverLogger.Error("Failed to secure log files during shutdown", zap.Error(err))
	}

	serverLogger.Info("Server exited successfully")
}
