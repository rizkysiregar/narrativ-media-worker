package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rizkysiregar/feedzmore-media-worker/internal/config"
	"github.com/rizkysiregar/feedzmore-media-worker/internal/storage"
	"github.com/rizkysiregar/feedzmore-media-worker/internal/thumbnail"
	"github.com/rizkysiregar/feedzmore-media-worker/internal/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	store, err := storage.NewMinio(cfg.MinioEndpoint, cfg.MinioSecure, cfg.MinioAccess, cfg.MinioSecret, cfg.MinioBucket)
	if err != nil {
		slog.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	consumer := worker.NewConsumer(cfg, worker.Processor{
		Store:     store,
		Extractor: thumbnail.FFmpeg{Path: cfg.FFmpegPath},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if consumer.Ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := &http.Server{Addr: cfg.HealthListenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("health server failed", "error", err)
		}
	}()

	slog.Info("media worker started")
	_ = consumer.Run(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	slog.Info("media worker stopped")
}
