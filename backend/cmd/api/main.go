// Command api serves the HTTP API. In AWS Lambda (AWS_LAMBDA_FUNCTION_NAME set) it runs behind
// API Gateway HTTP API (payload v2); locally it listens on HTTP_ADDR (default :8080).
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

	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"

	"athlix/backend/internal/app"
	"athlix/backend/internal/modules"
	"athlix/backend/internal/platform/config"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", slog.String("err", err.Error()))
		os.Exit(1)
	}
	d, cleanup, err := app.Bootstrap(ctx, cfg)
	if err != nil {
		slog.Error("bootstrap", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer cleanup()

	mods := modules.All(d)
	app.RegisterEventHandlers(d, mods) // local in-process bus; in AWS the worker handles events
	r := app.NewRouter(d, mods)

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(ginadapter.NewV2(r).ProxyWithContext)
		return
	}
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		d.Log.Info("api listening", slog.String("addr", cfg.HTTPAddr), slog.String("env", string(cfg.Env)))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			d.Log.Error("listen", slog.String("err", err.Error()))
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
}
