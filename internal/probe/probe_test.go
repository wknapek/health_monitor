package probe

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"ubi/internal/checksum"
	"ubi/internal/config"
	"ubi/internal/model"
	"ubi/internal/store"
	pb "ubi/proto/monitor/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestNewCollector(t *testing.T) {
	cfg := config.CollectorConfig{
		Interval:      10 * time.Millisecond,
		GRPCTimeout:   time.Second,
		RESTTimeout:   time.Second,
		MaxConcurrent: 2,
	}
	c := NewCollector(store.New(), cfg, discardLogger(), checksum.NewChecksumController())
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.interval != 10*time.Millisecond {
		t.Errorf("interval = %v, want 10ms", c.interval)
	}
	if c.maxCon != 2 {
		t.Errorf("maxCon = %d, want 2", c.maxCon)
	}
	if c.checksumCtr == nil {
		t.Error("checksumCtr should not be nil")
	}
}

func TestProbeAll_NoDevices(t *testing.T) {
	c := NewCollector(store.New(), config.CollectorConfig{MaxConcurrent: 1}, discardLogger(), nil)
	c.ProbeAll(t.Context())
}

// newTestSrv starts an httptest server that serves /health and /diagnostics
// and returns a pointer to the health request counter.
func newTestSrv(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var mu sync.Mutex
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":       "up",
				"capabilities": []string{"hw", "sw", "fw", "checksum", "status"},
			})
		case "/diagnostics":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"hw_version": "HW-1",
				"sw_version": "SW-1",
				"fw_version": "FW-1",
				"checksum":   "abc123",
				"status":     "up",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestProbe_GRPCDevice_Down(t *testing.T) {
	c := NewCollector(store.New(), config.CollectorConfig{GRPCTimeout: 300 * time.Millisecond, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.grpcProbe.Probe(t.Context(), "127.0.0.1:1", nil)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", res.Status)
	}
}

func TestProbe_RESTDevice_Up(t *testing.T) {
	srv, _ := newTestSrv(t)
	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("Status = %v, want UP", res.Status)
	}
	if res.Diagnostics == nil {
		t.Fatal("expected diagnostics")
	}
	if res.Diagnostics.HardwareVersion != "HW-1" ||
		res.Diagnostics.SoftwareVersion != "SW-1" ||
		res.Diagnostics.FirmwareVersion != "FW-1" ||
		res.Diagnostics.Checksum != "abc123" {
		t.Errorf("unexpected diagnostics: %+v", res.Diagnostics)
	}
}

func TestProbe_RESTDevice_Down(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: 200 * time.Millisecond, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", res.Status)
	}
}

func TestProbe_RESTDevice_ServingFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_serving"})
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", res.Status)
	}
	if res.Capabilities != nil {
		t.Errorf("Capabilities = %v, want nil for non-serving device", res.Capabilities)
	}
}

func TestMergeJSONDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]any
		want model.Diagnostics
	}{
		{
			name: "full names",
			m:    map[string]any{"hardware_version": "H", "software_version": "S", "firmware_version": "F", "checksum": "C"},
			want: model.Diagnostics{HardwareVersion: "H", SoftwareVersion: "S", FirmwareVersion: "F", Checksum: "C"},
		},
		{
			name: "short names",
			m:    map[string]any{"hw": "H2", "sw": "S2", "fw": "F2"},
			want: model.Diagnostics{HardwareVersion: "H2", SoftwareVersion: "S2", FirmwareVersion: "F2"},
		},
		{
			name: "hardware alias",
			m:    map[string]any{"hardware": "H3"},
			want: model.Diagnostics{HardwareVersion: "H3"},
		},
		{
			name: "status up",
			m:    map[string]any{"status": "up"},
			want: model.Diagnostics{Status: pb.DeviceStatus_DEVICE_STATUS_UP},
		},
		{
			name: "state down",
			m:    map[string]any{"state": "down"},
			want: model.Diagnostics{Status: pb.DeviceStatus_DEVICE_STATUS_DOWN},
		},
		{
			name: "unknown status ignored",
			m:    map[string]any{"status": "bogus"},
			want: model.Diagnostics{},
		},
		{
			name: "non-string values ignored",
			m:    map[string]any{"hw": 42, "status": true},
			want: model.Diagnostics{},
		},
		{
			name: "case insensitive keys",
			m:    map[string]any{"HW_VERSION": "H4"},
			want: model.Diagnostics{HardwareVersion: "H4"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &model.Diagnostics{}
			mergeJSONDiagnostics(d, c.m)
			if d.HardwareVersion != c.want.HardwareVersion ||
				d.SoftwareVersion != c.want.SoftwareVersion ||
				d.FirmwareVersion != c.want.FirmwareVersion ||
				d.Checksum != c.want.Checksum ||
				d.Status != c.want.Status {
				t.Errorf("got %+v, want %+v", d, c.want)
			}
		})
	}
}

func TestMergeDiagnostics(t *testing.T) {
	dst := &model.Diagnostics{SoftwareVersion: "keep"}
	src := &model.Diagnostics{HardwareVersion: "H", SoftwareVersion: "overwrite", Status: pb.DeviceStatus_DEVICE_STATUS_DOWN}
	mergeDiagnostics(dst, src)
	if dst.HardwareVersion != "H" {
		t.Errorf("HardwareVersion = %q, want H", dst.HardwareVersion)
	}
	if dst.SoftwareVersion != "overwrite" {
		t.Errorf("SoftwareVersion = %q, want overwrite", dst.SoftwareVersion)
	}
	if dst.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", dst.Status)
	}

	mergeDiagnostics(dst, nil)
	if dst.HardwareVersion != "H" {
		t.Errorf("merge with nil should be a no-op, got %+v", dst)
	}
}

func TestNormalizeStringStatus(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"up", "up"}, {"healthy", "up"}, {"ok", "up"}, {"serving", "up"},
		{"down", "down"}, {"unreachable", "down"}, {"not_serving", "down"},
		{"degraded", "degraded"}, {"unknown", "degraded"}, {"warning", "degraded"},
		{"", ""}, {"custom", "custom"},
	}
	for _, c := range cases {
		if got := normalizeStringStatus(c.in); got != c.want {
			t.Errorf("normalizeStringStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHealthResponseMethods(t *testing.T) {
	hr := &healthResponse{Status: "healthy", Capabilities: []string{"hw", "sw"}}
	if !hr.Serving() {
		t.Error("Serving() should be true")
	}
	if st := hr.StatusEnum(); st != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("StatusEnum() = %v, want UP", st)
	}
	if caps := hr.NormalizedCaps(); len(caps) != 2 {
		t.Errorf("NormalizedCaps() = %v, want 2 caps", caps)
	}
}

func TestParseStatus(t *testing.T) {
	cases := []struct {
		in   string
		want pb.DeviceStatus
	}{
		{"up", pb.DeviceStatus_DEVICE_STATUS_UP},
		{"healthy", pb.DeviceStatus_DEVICE_STATUS_UP},
		{"ok", pb.DeviceStatus_DEVICE_STATUS_UP},
		{"serving", pb.DeviceStatus_DEVICE_STATUS_UP},
		{"down", pb.DeviceStatus_DEVICE_STATUS_DOWN},
		{"unreachable", pb.DeviceStatus_DEVICE_STATUS_DOWN},
		{"not_serving", pb.DeviceStatus_DEVICE_STATUS_DOWN},
		{"degraded", pb.DeviceStatus_DEVICE_STATUS_DEGRADED},
		{"unknown", pb.DeviceStatus_DEVICE_STATUS_DEGRADED},
		{"warning", pb.DeviceStatus_DEVICE_STATUS_DEGRADED},
		{"bogus", pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := parseStatus(c.in); got != c.want {
			t.Errorf("parseStatus(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMapHealthStatus(t *testing.T) {
	cases := []struct {
		in   healthpb.HealthCheckResponse_ServingStatus
		want pb.DeviceStatus
	}{
		{healthpb.HealthCheckResponse_SERVING, pb.DeviceStatus_DEVICE_STATUS_UP},
		{healthpb.HealthCheckResponse_NOT_SERVING, pb.DeviceStatus_DEVICE_STATUS_DOWN},
		{healthpb.HealthCheckResponse_UNKNOWN, pb.DeviceStatus_DEVICE_STATUS_DEGRADED},
	}
	for _, c := range cases {
		if got := mapHealthStatus(c.in); got != c.want {
			t.Errorf("mapHealthStatus(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestMapStatusErr(t *testing.T) {
	if st := mapStatusErr(nil); st != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("mapStatusErr(nil) = %v, want UP", st)
	}
	unavailable := status.Error(codes.Unavailable, "down")
	if st := mapStatusErr(unavailable); st != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("mapStatusErr(unavailable) = %v, want DOWN", st)
	}
	unknown := status.Error(codes.Unknown, "boom")
	if st := mapStatusErr(unknown); st != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("mapStatusErr(unknown) = %v, want DEGRADED", st)
	}
}

func TestCollector_RESTNonJSONHealth(t *testing.T) {
	// Some devices return a bare "ok" body from /health that fails to decode.
	// The probe must not panic and must mark the device as not-serving.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("Status = %v, want DEGRADED (bare 'ok' body is not serving)", res.Status)
	}
}

func TestCollector_RESTPerCapabilityEndpoints(t *testing.T) {
	// /diagnostics returns non-200; the probe must fall back to per-cap endpoints.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "up", "capabilities": []string{"hw", "checksum"}})
		case "/diagnostics":
			http.Error(w, "not found", http.StatusNotFound)
		case "/diagnostics/hw":
			_ = json.NewEncoder(w).Encode(map[string]string{"hw_version": "H"})
		case "/diagnostics/checksum":
			_ = json.NewEncoder(w).Encode(map[string]string{"checksum": "C"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Diagnostics == nil {
		t.Fatal("expected diagnostics from per-capability endpoints")
	}
	if res.Diagnostics.HardwareVersion != "H" || res.Diagnostics.Checksum != "C" {
		t.Errorf("unexpected diagnostics: %+v", res.Diagnostics)
	}
}

func TestCollector_RESTStatusOverrides(t *testing.T) {
	// Diagnostics reporting a degraded status should downgrade the device.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "up", "capabilities": []string{"status"}})
		case "/diagnostics":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "degraded"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), srv.URL)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("Status = %v, want DEGRADED", res.Status)
	}
}

func TestCollectorRun_CancelsQuickly(t *testing.T) {
	srv, hits := newTestSrv(t)
	st := store.New()
	st.AddDevice(&model.Device{ID: "d1", Address: srv.URL, Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})

	cfg := config.CollectorConfig{Interval: 50 * time.Millisecond, RESTTimeout: time.Second, MaxConcurrent: 1}
	c := NewCollector(st, cfg, discardLogger(), nil)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
	if *hits < 1 {
		t.Errorf("expected at least one health hit, got %d", *hits)
	}
	if d, ok := st.GetDevice("d1"); !ok || d.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("device status not updated: %+v", d)
	}
	if diag, ok := st.GetDiagnostics("d1"); !ok || diag == nil {
		t.Error("diagnostics not stored")
	}
}

func TestCollector_RESTConnectError(t *testing.T) {
	c := NewCollector(store.New(), config.CollectorConfig{RESTTimeout: 100 * time.Millisecond, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.restProbe.Probe(t.Context(), "http://127.0.0.1:1")
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", res.Status)
	}
}

func TestCollector_ProbeAllPersists(t *testing.T) {
	srv, _ := newTestSrv(t)
	st := store.New()
	st.AddDevice(&model.Device{ID: "d1", Address: srv.URL, Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})

	c := NewCollector(st, config.CollectorConfig{RESTTimeout: time.Second, MaxConcurrent: 2}, discardLogger(), nil)
	c.ProbeAll(t.Context())

	d, ok := st.GetDevice("d1")
	if !ok {
		t.Fatal("device missing after probe")
	}
	if d.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("Status = %v, want UP", d.Status)
	}
	if len(d.Capabilities) == 0 {
		t.Error("capabilities should be set")
	}
	if _, ok := st.GetDiagnostics("d1"); !ok {
		t.Error("diagnostics should be stored")
	}
}

// ---------- gRPC probe integration tests ----------

// mockHealthServer implements the standard gRPC health service for the test.
type mockHealthServer struct {
	healthpb.UnimplementedHealthServer
	diagService string
	serving     bool
}

func (s *mockHealthServer) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if req.GetService() == "" {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}
	if s.serving && req.GetService() == s.diagService {
		return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING}, nil
}

// mockDiagServer implements the DeviceDiagnostics.Collect RPC.
type mockDiagServer struct {
	pb.UnimplementedDeviceDiagnosticsServer
	hw, sw, fw, checksum string
	status               pb.DeviceStatus
}

func (s *mockDiagServer) Collect(ctx context.Context, req *pb.CollectDiagnosticsRequest) (*pb.CollectDiagnosticsResponse, error) {
	return &pb.CollectDiagnosticsResponse{
		HardwareVersion: s.hw,
		SoftwareVersion: s.sw,
		FirmwareVersion: s.fw,
		Checksum:        s.checksum,
		Status:          s.status,
	}, nil
}

// startGRPCMockDevice starts an in-process gRPC server that advertises the
// diagnostics service and returns its address.
func startGRPCMockDevice(t *testing.T, withDiagService bool) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	g := grpc.NewServer()
	healthpb.RegisterHealthServer(g, &mockHealthServer{diagService: deviceDiagnosticsService, serving: withDiagService})
	if withDiagService {
		pb.RegisterDeviceDiagnosticsServer(g, &mockDiagServer{
			hw: "HW-G", sw: "SW-G", fw: "FW-G", checksum: "csum-g", status: pb.DeviceStatus_DEVICE_STATUS_DEGRADED,
		})
	}
	go func() {
		_ = g.Serve(lis)
	}()
	t.Cleanup(g.Stop)
	return lis.Addr().String()
}

func TestProbe_GRPCDevice_UpWithDiagnostics(t *testing.T) {
	addr := startGRPCMockDevice(t, true)
	c := NewCollector(store.New(), config.CollectorConfig{GRPCTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.grpcProbe.Probe(t.Context(), addr, nil)

	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("Status = %v, want DEGRADED (from diagnostics)", res.Status)
	}
	if len(res.Capabilities) != len(supportedCaps) {
		t.Errorf("len(Capabilities) = %d, want %d", len(res.Capabilities), len(supportedCaps))
	}
	if res.Diagnostics == nil {
		t.Fatal("expected diagnostics")
	}
	if res.Diagnostics.HardwareVersion != "HW-G" ||
		res.Diagnostics.SoftwareVersion != "SW-G" ||
		res.Diagnostics.FirmwareVersion != "FW-G" ||
		res.Diagnostics.Checksum != "csum-g" {
		t.Errorf("unexpected diagnostics: %+v", res.Diagnostics)
	}
}

func TestProbe_GRPCDevice_NoDiagService(t *testing.T) {
	addr := startGRPCMockDevice(t, false)
	c := NewCollector(store.New(), config.CollectorConfig{GRPCTimeout: time.Second, MaxConcurrent: 1}, discardLogger(), nil)

	// No capabilities advertised; with existing caps the probe retains them.
	res := c.grpcProbe.Probe(t.Context(), addr, []string{"hw", "sw"})
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("Status = %v, want UP", res.Status)
	}
	if len(res.Capabilities) != 2 {
		t.Errorf("len(Capabilities) = %d, want 2 (existing retained)", len(res.Capabilities))
	}
	if res.Diagnostics != nil {
		t.Error("Diagnostics should be nil when no diag service advertised")
	}
}

func TestProbe_GRPCDevice_Unreachable(t *testing.T) {
	c := NewCollector(store.New(), config.CollectorConfig{GRPCTimeout: 200 * time.Millisecond, MaxConcurrent: 1}, discardLogger(), nil)
	res := c.grpcProbe.Probe(t.Context(), "127.0.0.1:1", nil)
	if res.Status != pb.DeviceStatus_DEVICE_STATUS_DOWN {
		t.Errorf("Status = %v, want DOWN", res.Status)
	}
}

func TestDiscoverCapabilities(t *testing.T) {
	addr := startGRPCMockDevice(t, true)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	caps := discoverCapabilities(t.Context(), conn, nil)
	if len(caps) != len(supportedCaps) {
		t.Errorf("len(caps) = %d, want %d", len(caps), len(supportedCaps))
	}

	connNoDiag, err := grpc.NewClient(startGRPCMockDevice(t, false), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connNoDiag.Close()
	caps = discoverCapabilities(t.Context(), connNoDiag, []string{"hw"})
	if len(caps) != 1 || caps[0] != "hw" {
		t.Errorf("caps = %v, want existing [hw]", caps)
	}
}
