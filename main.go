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

	"timoruohomaki/http-logging-framework/middleware/logging"
)

func main() {
	// Create a development logger for server startup/shutdown messages
	serverLogger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Printf("Failed to create server logger: %v\n", err)
		os.Exit(1)
	}
	defer serverLogger.Sync()

	// Create Apache CLF logger with rotation
	config := logging.DefaultApacheLogConfig()

	// You can customize the config if needed
	// config.LogPath = "/custom/path/access.log"
	// config.MaxSize = 50

	accessLogger, err := logging.NewApacheLogger(config)
	if err != nil {
		serverLogger.Fatal("Failed to create access logger", zap.Error(err))
	}
	defer accessLogger.Sync()

	// Define API routes
	mux := http.NewServeMux()

	// Example route
	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Hello, World!"}`))
	})

	// Add the Apache Common Log Format middleware
	handler := logging.ApacheCommonLogMiddleware(accessLogger)(mux)

	// Configure the HTTP server
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	// Run server in a goroutine so we can gracefully handle shutdown
	go func() {
		serverLogger.Info("Starting server", zap.String("address", ":8080"))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverLogger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	serverLogger.Info("Shutting down server...")

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		serverLogger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	serverLogger.Info("Server exited successfully")
}
