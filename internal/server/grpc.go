package server

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"ubi/internal/service"
	pb "ubi/proto/monitor/v1"
)

type GRPCServer struct {
	pb.UnimplementedMonitorServiceServer
	svc    *service.Service
	logger *slog.Logger
}

func NewGRPCServer(svc *service.Service, logger *slog.Logger) *GRPCServer {
	return &GRPCServer{svc: svc, logger: logger}
}

func (s *GRPCServer) ListDevices(ctx context.Context, req *pb.ListDevicesRequest) (*pb.ListDevicesResponse, error) {
	return &pb.ListDevicesResponse{Devices: s.svc.ListDevices(ctx)}, nil
}

func (s *GRPCServer) GetDevice(ctx context.Context, req *pb.GetDeviceRequest) (*pb.GetDeviceResponse, error) {
	d, err := s.svc.GetDevice(ctx, req.GetDeviceId())
	if err != nil {
		s.logger.Warn("device not found", "id", req.GetDeviceId(), "error", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &pb.GetDeviceResponse{Device: d}, nil
}

func (s *GRPCServer) GetDiagnostics(ctx context.Context, req *pb.GetDiagnosticsRequest) (*pb.GetDiagnosticsResponse, error) {
	d, err := s.svc.GetDiagnostics(ctx, req.GetDeviceId())
	if err != nil {
		s.logger.Warn("diagnostics not found", "id", req.GetDeviceId(), "error", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &pb.GetDiagnosticsResponse{Diagnostics: d}, nil
}

func (s *GRPCServer) AddDevice(ctx context.Context, req *pb.AddDeviceRequest) (*pb.AddDeviceResponse, error) {
	d, err := s.svc.AddDevice(ctx, req.GetId(), req.GetName(), req.GetAddress(), req.GetProtocol())
	if err != nil {
		s.logger.Warn("failed to add device", "id", req.GetId(), "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	s.logger.Info("device added",
		"id", req.GetId(), "name", req.GetName(),
		"address", req.GetAddress(), "protocol", req.GetProtocol().String())
	return &pb.AddDeviceResponse{Device: d}, nil
}

func (s *GRPCServer) RemoveDevice(ctx context.Context, req *pb.RemoveDeviceRequest) (*pb.RemoveDeviceResponse, error) {
	if err := s.svc.RemoveDevice(ctx, req.GetDeviceId()); err != nil {
		s.logger.Warn("failed to remove device", "id", req.GetDeviceId(), "error", err)
		return nil, status.Error(codes.NotFound, err.Error())
	}
	s.logger.Info("device removed", "id", req.GetDeviceId())
	return &pb.RemoveDeviceResponse{}, nil
}

// Register registers the monitor + health services on the gRPC server.
func (s *GRPCServer) Register(g *grpc.Server) {
	pb.RegisterMonitorServiceServer(g, s)
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(g, healthSrv)
	reflection.Register(g)
}
