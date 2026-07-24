package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/RinTanth/go-backend/config"
	"github.com/RinTanth/go-backend/router"
	"github.com/RinTanth/go-common/logger"
	"github.com/RinTanth/go-common/shutdown"

	_ "embed"
	_ "time/tzdata"
)

const (
	gracefulShutdownDuration = 10 * time.Second
	serverReadHeaderTimeout  = 5 * time.Second
	serverReadTimeout        = 5 * time.Second
	serverWriteTimeout       = 10 * time.Second // request hangup after this durations
	handlerTimeout           = serverWriteTimeout - (time.Millisecond * 100)
)

// go build -ldflags "-X main.commit=123456"
var commit string

//go:embed VERSION
var version string

func main() {
	cfg := config.C(config.Env)
	_ = logger.New(logger.GCPKeyReplacer)

	r, stop := router.New(cfg, version, commit, handlerTimeout)
	defer stop()

	srv := newServer(cfg, r)

	go shutdown.Graceful(srv, gracefulShutdownDuration)

	fmt.Printf("\n🚀 Server running on Port:%s\n\n", cfg.Server.Port)
	slog.Info("run", "port", cfg.Server.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("HTTP server ListenAndServe", "error", err)
		os.Exit(1)
	}

	slog.Info("bye")
}

func newServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		MaxHeaderBytes:    1 << 20,
	}
}
