package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-chi-observability/internal/observability"
	"go-chi-observability/internal/server"
)

func main() {

	ctx := context.Background()

	// Logger
	err := observability.InitLogger()
	if err != nil {
		panic(err)
	}
	defer observability.SyncLogger()

	// Tracing
	traceShutdown, err := observability.InitTracing(ctx)
	if err != nil {
		panic(err)
	}
	defer traceShutdown(ctx)

	// Metrics
	metricShutdown, err := initMetrics(ctx)
	if err != nil {
		panic(err)
	}
	defer metricShutdown(ctx)

	// Router
	router := server.NewRouter()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		observability.Logger.Info("server started")

		if err := srv.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	waitForShutdown(srv)
}

func waitForShutdown(srv *http.Server) {

	stop := make(chan os.Signal, 1)

	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	srv.Shutdown(ctx)
}
