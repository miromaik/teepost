package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"teepost/frontend"
	"teepost/internal/db"
	"teepost/internal/media"
	"teepost/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := server.LoadConfig()

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open sqlite database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if cErr := store.Close(); cErr != nil {
			slog.Error("failed to close database", "error", cErr)
		}
	}()

	mediaMgr, err := media.NewManager(cfg.MediaDir)
	if err != nil {
		slog.Error("failed to initialize media storage", "error", err)
		os.Exit(1)
	}

	distFS, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		slog.Error("failed to mount embedded frontend assets", "error", err)
		os.Exit(1)
	}

	srv := server.New(cfg, store, mediaMgr, distFS)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go srv.StartScheduler(ctx)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	slog.Info("Teepost is running",
		"url", "http://localhost:"+cfg.Port,
		"dbPath", cfg.DBPath,
		"mediaDir", cfg.MediaDir,
	)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if sErr := httpServer.Shutdown(shutdownCtx); sErr != nil {
			slog.Error("graceful shutdown error", "error", sErr)
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server error", "error", err)
		os.Exit(1)
	}
}
