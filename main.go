package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/spendesk/github-actions-exporter/pkg/config"
	"github.com/spendesk/github-actions-exporter/pkg/server"
)

var (
	version = "development"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app := cli.NewApp()
	app.Name = "github-actions-exporter"
	app.Flags = config.InitConfiguration()
	app.Version = version
	app.Action = func(c *cli.Context) error {
		server.Version = version
		return server.RunServer(ctx)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
