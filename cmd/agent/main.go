package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"caichip/internal/agentapp"
)

func main() {
	cfg, err := agentapp.LoadConfig()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	closeLog, err := agentapp.SetupLogging(cfg)
	if err != nil {
		slog.Error("log setup", "err", err)
		os.Exit(1)
	}
	defer closeLog()

	if err := agentapp.VerifyPythonAndPip(cfg); err != nil {
		slog.Error("python/pip 检查失败", "err", err)
		os.Exit(1)
	}

	app, err := agentapp.NewApp(context.Background(), cfg)
	if err != nil {
		slog.Error("agent client", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		slog.Error("agent", "err", err)
		os.Exit(1)
	}
}
