package main

import (
	"flag"
	"os"

	"caichip/internal/conf"
	"caichip/internal/pkg/logx"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
)

var flagConf string

type runtimeBootstrap struct {
	Log logx.Config `json:"log" yaml:"log"`
}

func init() {
	flag.StringVar(&flagConf, "conf", "D:\\workspace\\caichip\\configs\\config.yaml", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()
	os.Setenv("CAICHIP_BOM_MATCH_TIMING", "1")
	logger := log.NewStdLogger(os.Stdout)

	c := config.New(
		config.WithSource(
			file.NewSource(flagConf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		log.NewHelper(logger).Fatalf("load config: %v", err)
	}

	var runtimeCfg runtimeBootstrap
	if err := c.Scan(&runtimeCfg); err != nil {
		log.NewHelper(logger).Fatalf("scan runtime config: %v", err)
	}
	zapLogger, err := logx.NewZapLoggerWithConfig(runtimeCfg.Log)
	if err != nil {
		log.NewHelper(logger).Fatalf("init zap logger: %v", err)
	}
	defer func() { _ = zapLogger.Sync() }()
	logger = log.With(zapLogger, "ts", log.DefaultTimestamp, "caller", log.DefaultCaller)
	log.SetLogger(logger)

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		log.NewHelper(logger).Fatalf("scan config: %v", err)
	}
	var px conf.BootstrapProxy
	if err := c.Scan(&px); err != nil {
		log.NewHelper(logger).Fatalf("scan proxy config: %v", err)
	}

	app, cleanup, err := wireApp(&bc, &px, logger)
	if err != nil {
		log.NewHelper(logger).Fatalf("wire app: %v", err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		log.NewHelper(logger).Fatalf("run app: %v", err)
	}
}
