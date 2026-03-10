package server

import (
	"context"
	"log"
	"strconv"

	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"

	"github.com/spendesk/github-actions-exporter/pkg/config"
	"github.com/spendesk/github-actions-exporter/pkg/metrics"
)

var Version string

func RunServer(ctx context.Context) error {
	metrics.InitMetrics(ctx, Version)

	r := router.New()
	r.GET("/", func(ctx *fasthttp.RequestCtx) {
		ctx.WriteString("/metrics")
	})
	r.GET("/metrics", prometheusHandler())
	r.GET("/health", func(ctx *fasthttp.RequestCtx) {
		ctx.SetContentType("application/json")
		ctx.WriteString(`{"status":"ok"}`)
	})

	if config.Debug {
		r.GET("/debug/pprof/", pprofHandlerIndex)
		r.GET("/debug/pprof/cmdline", pprofHandlerCmdline)
		r.GET("/debug/pprof/profile", pprofHandlerIndex)
		r.GET("/debug/pprof/trace", pprofHandlerTrace)
		r.GET("/debug/pprof/{profile}", pprofHandlerIndex)
	}

	server := &fasthttp.Server{
		Handler: r.Handler,
	}

	go func() {
		<-ctx.Done()
		log.Print("shutting down HTTP server...")
		server.Shutdown()
	}()

	addr := ":" + strconv.Itoa(config.Port)
	log.Print("exporter listening on 0.0.0.0" + addr)
	return server.ListenAndServe(addr)
}
