// Command hub starts the Claude Room Hub server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/liuwenchang/claude-room/internal/hub"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:7777", "listen address")
	token := flag.String("token", "", "optional auth token")
	histSize := flag.Int("history", 200, "per-room message history size")
	flag.Parse()

	// Support YAML config via env override.
	if v := os.Getenv("CLAUDE_ROOM_ADDR"); v != "" {
		*addr = v
	}
	if v := os.Getenv("CLAUDE_ROOM_TOKEN"); v != "" {
		*token = v
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	srv := hub.NewServer(hub.Config{
		ListenAddr:  *addr,
		AuthToken:   *token,
		HistorySize: *histSize,
	})

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	go func() {
		slog.Info("hub listening", "addr", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("hub serve error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down hub")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
