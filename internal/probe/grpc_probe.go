package probe

import (
	"context"
	"log/slog"
	"time"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// deviceDiagnosticsService is the fully-qualified name of the convention that
// gRPC devices implement to expose their diagnostics.
const deviceDiagnosticsService = "device.Diagnostics"

type grpcProbe struct {
	timeout time.Duration
	logger  *slog.Logger
}

func newGRPCProbe(timeout time.Duration, logger *slog.Logger) *grpcProbe {
	return &grpcProbe{timeout: timeout, logger: logger}
}

func (p *grpcProbe) Probe(ctx context.Context, addr string, existingCaps []string) *model.ProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		p.logger.Warn("failed to dial gRPC device", "address", addr, "error", err)
		return &model.ProbeResult{Status: pb.DeviceStatus_DEVICE_STATUS_DOWN}
	}
	defer conn.Close()

	healthClient := healthpb.NewHealthClient(conn)
	var resp *healthpb.HealthCheckResponse
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = healthClient.Check(probeCtx, &healthpb.HealthCheckRequest{})
		if err == nil {
			break
		}
		p.logger.Info("failed to check health", "address", addr, "err", err)
		p.logger.Debug("retrying health check", "address", addr, "attempt", attempt+1)
	}
	if err != nil {
		p.logger.Warn("gRPC device unhealthy", "address", addr, "status", mapStatusErr(err).String(), "error", err)
		return &model.ProbeResult{Status: mapStatusErr(err)}
	}

	p.logger.Debug("gRPC health check ok", "address", addr, "status", resp.GetStatus().String())

	r := &model.ProbeResult{Status: mapHealthStatus(resp.GetStatus())}

	caps := discoverCapabilities(probeCtx, conn, existingCaps)
	r.Capabilities = caps
	p.logger.Debug("gRPC capabilities", "address", addr, "capabilities", caps)

	if diag := p.collectDiagnostics(probeCtx, conn, caps); diag != nil {
		r.Diagnostics = diag
		p.logger.Debug("gRPC diagnostics collected", "address", addr,
			"hw", diag.HardwareVersion, "sw", diag.SoftwareVersion, "fw", diag.FirmwareVersion)
		if diag.Status != pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED {
			r.Status = diag.Status
		}
	} else {
		p.logger.Debug("no gRPC diagnostics collected", "address", addr, "capabilities", caps)
	}

	return r
}

// discoverCapabilities determines which diagnostics the device supports. For
// gRPC devices this relies on the standard health check of the diagnostics
// service; if unadvertised, it falls back to previously observed capabilities.
func discoverCapabilities(ctx context.Context, conn *grpc.ClientConn, existing []string) []string {
	client := healthpb.NewHealthClient(conn)
	if resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: deviceDiagnosticsService}); err == nil &&
		resp.GetStatus() == healthpb.HealthCheckResponse_SERVING {
		return supportedCaps
	}
	return existing
}

// collectDiagnostics invokes the device's DeviceDiagnostics.Collect RPC using
// the typed generated client and maps the result into the common model.
func (p *grpcProbe) collectDiagnostics(ctx context.Context, conn *grpc.ClientConn, caps []string) *model.Diagnostics {
	if len(caps) == 0 {
		return nil
	}

	client := pb.NewDeviceDiagnosticsClient(conn)
	resp, err := client.Collect(ctx, &pb.CollectDiagnosticsRequest{})
	if err != nil {
		p.logger.Debug("device diagnostics collect failed", "error", err)
		return nil
	}

	return &model.Diagnostics{
		HardwareVersion: resp.GetHardwareVersion(),
		SoftwareVersion: resp.GetSoftwareVersion(),
		FirmwareVersion: resp.GetFirmwareVersion(),
		Status:          resp.GetStatus(),
		Checksum:        resp.GetChecksum(),
		CollectedAt:     time.Now(),
	}
}

var supportedCaps = []string{"hw", "sw", "fw", "checksum", "status"}

// parseStatus maps a string status from a device's diagnostic payload to the
// canonical DeviceStatus enum.
func parseStatus(s string) pb.DeviceStatus {
	switch s {
	case "up", "healthy", "ok", "serving":
		return pb.DeviceStatus_DEVICE_STATUS_UP
	case "down", "unreachable", "not_serving":
		return pb.DeviceStatus_DEVICE_STATUS_DOWN
	case "degraded", "unknown", "warning":
		return pb.DeviceStatus_DEVICE_STATUS_DEGRADED
	default:
		return pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED
	}
}

func mapHealthStatus(h healthpb.HealthCheckResponse_ServingStatus) pb.DeviceStatus {
	switch h {
	case healthpb.HealthCheckResponse_SERVING:
		return pb.DeviceStatus_DEVICE_STATUS_UP
	case healthpb.HealthCheckResponse_NOT_SERVING:
		return pb.DeviceStatus_DEVICE_STATUS_DOWN
	default:
		return pb.DeviceStatus_DEVICE_STATUS_DEGRADED
	}
}

func mapStatusErr(err error) pb.DeviceStatus {
	if err == nil {
		return pb.DeviceStatus_DEVICE_STATUS_UP
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded:
			return pb.DeviceStatus_DEVICE_STATUS_DOWN
		}
	}
	return pb.DeviceStatus_DEVICE_STATUS_DEGRADED
}
