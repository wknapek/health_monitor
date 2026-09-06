package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	pb "ubi/proto/monitor/v1"
)

// mock device implementing both the REST health/capability convention and
// the gRPC health + Diagnostics convention used by the monitor.

var (
	hwVersion = "REV-3.2"
	swVersion = "12.4.5"
	fwVersion = "2.1.0"
	checksum  = "abc123def456"
	state     = "up"
)

const (
	diagnosticsService = "device.Diagnostics"
)

func main() {
	var mode string
	flag.StringVar(&mode, "mode", "rest", "device mode: rest or grpc")
	flag.Parse()

	switch mode {
	case "grpc":
		runGRPC()
	default:
		runREST()
	}
}

// ---------- REST device ----------

func runREST() {
	addr := ":18080"
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"status":       "healthy",
			"capabilities": []string{"hw", "sw", "fw", "checksum", "status"},
		})
	})
	http.HandleFunc("/diagnostics", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"hw_version": hwVersion,
			"sw_version": swVersion,
			"fw_version": fwVersion,
			"status":     state,
			"checksum":   checksum,
		})
	})
	log.Printf("mock REST device listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ---------- gRPC device ----------

type deviceServer struct {
	healthpb.UnimplementedHealthServer
}

func (s *deviceServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if req.GetService() == "" {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}
	// Advertise the diagnostics service so the monitor discovers it via the
	// standard gRPC health check.
	if req.GetService() == diagnosticsService {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_UNKNOWN}, nil
}

func (s *deviceServer) Watch(req *healthpb.HealthCheckRequest, stream healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "watch not supported")
}

type diagServer struct {
	pb.UnimplementedDeviceDiagnosticsServer
}

func (s *diagServer) Collect(ctx context.Context, req *pb.CollectDiagnosticsRequest) (*pb.CollectDiagnosticsResponse, error) {
	return &pb.CollectDiagnosticsResponse{
		HardwareVersion: hwVersion,
		SoftwareVersion: swVersion,
		FirmwareVersion: fwVersion,
		Checksum:        checksum,
		Status:          pb.DeviceStatus_DEVICE_STATUS_UP,
	}, nil
}

func runGRPC() {
	addr := ":50051"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	g := grpc.NewServer()
	healthpb.RegisterHealthServer(g, &deviceServer{})
	pb.RegisterDeviceDiagnosticsServer(g, &diagServer{})

	log.Printf("mock gRPC device listening on %s", addr)
	go func() {
		if err := g.Serve(lis); err != nil {
			log.Fatalf("serve: %v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	<-c
	g.GracefulStop()
}
