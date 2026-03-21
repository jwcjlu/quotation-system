package main

import (
	"flag"
	"os"

	"caichip/internal/conf"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
)

var flagConf string

func init() {
	flag.StringVar(&flagConf, "conf", "D:\\workspace\\caichip\\configs\\config.yaml", "config path, eg: -conf config.yaml")
}

func main() {
	flag.Parse()

	logger := log.NewStdLogger(os.Stdout)
	log.SetLogger(logger)

	c := config.New(
		config.WithSource(
			file.NewSource(flagConf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		log.NewHelper(logger).Fatalf("load config: %v", err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		log.NewHelper(logger).Fatalf("scan config: %v", err)
	}

	app, cleanup, err := wireApp(&bc, logger)
	if err != nil {
		log.NewHelper(logger).Fatalf("wire app: %v", err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		log.NewHelper(logger).Fatalf("run app: %v", err)
	}
}
