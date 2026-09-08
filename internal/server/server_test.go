package server

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"ubi/internal/model"
	"ubi/internal/service"
	"ubi/internal/store"
	pb "ubi/proto/monitor/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestSetup(t *testing.T) (*service.Service, *store.Store) {
	t.Helper()
	st := store.New()
	logger := discardLogger()
	return service.New(st, logger), st
}

func addTestDevice(t *testing.T, st *store.Store, id, addr string) {
	t.Helper()
	st.AddDevice(&model.Device{
		ID:       id,
		Name:     "Test " + id,
		Address:  addr,
		Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC,
	})
}

// newGRPCServerTest starts an in-process gRPC server running the full
// server stack (logging interceptor included) and returns a client.
func newGRPCServerTest(t *testing.T, svc *service.Service) pb.MonitorServiceClient {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := grpc.NewServer(grpc.ChainUnaryInterceptor(UnaryLoggingInterceptor(discardLogger())))
	s := NewGRPCServer(svc, discardLogger())
	s.Register(g)
	go func() {
		_ = g.Serve(lis)
	}()
	t.Cleanup(g.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewMonitorServiceClient(conn)
}

func TestGRPC_ListDevices(t *testing.T) {
	svc, st := newTestSetup(t)
	addTestDevice(t, st, "dev-1", "10.0.0.1")
	addTestDevice(t, st, "dev-2", "10.0.0.2")

	client := newGRPCServerTest(t, svc)
	resp, err := client.ListDevices(t.Context(), &pb.ListDevicesRequest{})
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}
	if len(resp.Devices) != 2 {
		t.Errorf("len(Devices) = %d, want 2", len(resp.Devices))
	}
}

func TestGRPC_GetDevice(t *testing.T) {
	svc, st := newTestSetup(t)
	addTestDevice(t, st, "dev-1", "10.0.0.1")

	client := newGRPCServerTest(t, svc)
	resp, err := client.GetDevice(t.Context(), &pb.GetDeviceRequest{DeviceId: "dev-1"})
	if err != nil {
		t.Fatalf("GetDevice() error = %v", err)
	}
	if resp.GetDevice().GetId() != "dev-1" {
		t.Errorf("Id = %q, want dev-1", resp.GetDevice().GetId())
	}

	_, err = client.GetDevice(t.Context(), &pb.GetDeviceRequest{DeviceId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("GetDevice(missing) code = %v, want NotFound", status.Code(err))
	}
}

func TestGRPC_GetDiagnostics(t *testing.T) {
	svc, st := newTestSetup(t)
	addTestDevice(t, st, "dev-1", "10.0.0.1")
	st.SetDiagnostics("dev-1", &model.Diagnostics{
		DeviceID:        "dev-1",
		HardwareVersion: "HW",
		SoftwareVersion: "SW",
		Checksum:        "abc",
		Status:          pb.DeviceStatus_DEVICE_STATUS_UP,
	})

	client := newGRPCServerTest(t, svc)
	resp, err := client.GetDiagnostics(t.Context(), &pb.GetDiagnosticsRequest{DeviceId: "dev-1"})
	if err != nil {
		t.Fatalf("GetDiagnostics() error = %v", err)
	}
	if resp.GetDiagnostics().GetHardwareVersion() != "HW" {
		t.Errorf("HardwareVersion = %q, want HW", resp.GetDiagnostics().GetHardwareVersion())
	}

	_, err = client.GetDiagnostics(t.Context(), &pb.GetDiagnosticsRequest{DeviceId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("GetDiagnostics(missing) code = %v, want NotFound", status.Code(err))
	}
}

func TestGRPC_AddDevice(t *testing.T) {
	svc, _ := newTestSetup(t)
	client := newGRPCServerTest(t, svc)

	resp, err := client.AddDevice(t.Context(), &pb.AddDeviceRequest{
		Id:       "dev-9",
		Name:     "Nine",
		Address:  "10.0.0.9",
		Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST,
	})
	if err != nil {
		t.Fatalf("AddDevice() error = %v", err)
	}
	if resp.GetDevice().GetId() != "dev-9" {
		t.Errorf("Id = %q, want dev-9", resp.GetDevice().GetId())
	}

	// Duplicate -> InvalidArgument.
	_, err = client.AddDevice(t.Context(), &pb.AddDeviceRequest{
		Id: "dev-9", Address: "10.0.0.9", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("AddDevice(dup) code = %v, want InvalidArgument", status.Code(err))
	}

	// Invalid protocol -> InvalidArgument.
	_, err = client.AddDevice(t.Context(), &pb.AddDeviceRequest{
		Id: "dev-10", Address: "10.0.0.10", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("AddDevice(bad protocol) code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGRPC_RemoveDevice(t *testing.T) {
	svc, st := newTestSetup(t)
	addTestDevice(t, st, "dev-1", "10.0.0.1")
	client := newGRPCServerTest(t, svc)

	if _, err := client.RemoveDevice(t.Context(), &pb.RemoveDeviceRequest{DeviceId: "dev-1"}); err != nil {
		t.Fatalf("RemoveDevice() error = %v", err)
	}
	if _, ok := st.GetDevice("dev-1"); ok {
		t.Error("device still present after removal")
	}

	_, err := client.RemoveDevice(t.Context(), &pb.RemoveDeviceRequest{DeviceId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("RemoveDevice(missing) code = %v, want NotFound", status.Code(err))
	}
}

func TestGRPC_Health(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	svc, _ := newTestSetup(t)
	NewGRPCServer(svc, discardLogger()).Register(g)
	go func() { _ = g.Serve(lis) }()
	t.Cleanup(g.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	hc := healthpb.NewHealthClient(conn)
	resp, err := hc.Check(t.Context(), &healthpb.HealthCheckRequest{Service: ""})
	if err != nil {
		t.Fatalf("health check error = %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("Status = %v, want SERVING", resp.GetStatus())
	}
}

func TestInterceptor_LogsThroughToHandler(t *testing.T) {
	interceptor := UnaryLoggingInterceptor(discardLogger())
	var called bool
	handler := func(ctx context.Context, req any) (any, error) {
		called = true
		return "ok", nil
	}
	resp, err := interceptor(t.Context(), struct{}{}, &grpc.UnaryServerInfo{FullMethod: "/test/Method"}, handler)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}
}
