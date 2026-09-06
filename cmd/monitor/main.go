package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"ubi/internal/config"
	"ubi/internal/model"
	"ubi/internal/probe"
	"ubi/internal/server"
	"ubi/internal/service"
	"ubi/internal/store"
	pb "ubi/proto/monitor/v1"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var configPath string
	flag.StringVar(&configPath, "config", "configs/devices.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("starting device monitor",
		"version", version, "commit", commit, "built", date,
		"grpc", cfg.Server.GRPCAddr, "rest", cfg.Server.RESTAddr)

	st := store.New()

	// Seed initial devices from config.
	for _, d := range cfg.Devices {
		st.AddDevice(&model.Device{
			ID:       d.ID,
			Name:     d.Name,
			Address:  d.Address,
			Protocol: d.Protocol.ToProto(),
			Status:   pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED,
		})
	}
	logger.Info("loaded devices", "count", len(cfg.Devices))

	svc := service.New(st, logger)

	// Background collector.
	collector := probe.NewCollector(st, cfg.Collector, logger)
	collectorCtx, cancelCollector := context.WithCancel(context.Background())
	defer cancelCollector()
	go collector.Run(collectorCtx)

	// gRPC server.
	grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(server.UnaryLoggingInterceptor(logger)))
	srv := server.NewGRPCServer(svc, logger)
	srv.Register(grpcSrv)

	grpcLis, err := net.Listen("tcp", cfg.Server.GRPCAddr)
	if err != nil {
		logger.Error("failed to listen gRPC", "addr", cfg.Server.GRPCAddr, "error", err)
		os.Exit(1)
	}
	go func() {
		logger.Info("gRPC server listening", "addr", cfg.Server.GRPCAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			logger.Error("gRPC server error", "error", err)
			cancelCollector()
		}
	}()

	// REST server.
	restSrv := &http.Server{
		Addr:              cfg.Server.RESTAddr,
		Handler:           server.NewRESTServer(svc, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("REST server listening", "addr", cfg.Server.RESTAddr)
		if err := restSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("REST server error", "error", err)
			cancelCollector()
		}
	}()

	// Wait for shutdown signal.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down...")

	cancelCollector()
	grpcSrv.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = restSrv.Shutdown(shutdownCtx)

	logger.Info("shutdown complete")
}
