package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "ubi/proto/monitor/v1"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.GRPCAddr != ":9090" {
		t.Errorf("GRPCAddr = %q, want :9090", cfg.Server.GRPCAddr)
	}
	if cfg.Server.RESTAddr != ":8080" {
		t.Errorf("RESTAddr = %q, want :8080", cfg.Server.RESTAddr)
	}
	if cfg.Collector.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", cfg.Collector.Interval)
	}
	if cfg.Collector.GRPCTimeout != 5*time.Second {
		t.Errorf("GRPCTimeout = %v, want 5s", cfg.Collector.GRPCTimeout)
	}
	if cfg.Collector.RESTTimeout != 5*time.Second {
		t.Errorf("RESTTimeout = %v, want 5s", cfg.Collector.RESTTimeout)
	}
	if cfg.Collector.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want 10", cfg.Collector.MaxConcurrent)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeTemp(t, `
server:
  grpc_addr: ":10000"
  rest_addr: ":10001"
collector:
  interval: 10s
  grpc_timeout: 2s
  rest_timeout: 3s
  max_concurrent: 4
db:
  driver: postgres
  dsn: "host=db port=5432"
devices:
  - id: dev-1
    name: Device One
    address: 10.0.0.1:50051
    protocol: grpc
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.GRPCAddr != ":10000" {
		t.Errorf("GRPCAddr = %q, want :10000", cfg.Server.GRPCAddr)
	}
	if cfg.Server.RESTAddr != ":10001" {
		t.Errorf("RESTAddr = %q, want :10001", cfg.Server.RESTAddr)
	}
	if cfg.Collector.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want 10s", cfg.Collector.Interval)
	}
	if cfg.Collector.GRPCTimeout != 2*time.Second {
		t.Errorf("GRPCTimeout = %v, want 2s", cfg.Collector.GRPCTimeout)
	}
	if cfg.Collector.RESTTimeout != 3*time.Second {
		t.Errorf("RESTTimeout = %v, want 3s", cfg.Collector.RESTTimeout)
	}
	if cfg.Collector.MaxConcurrent != 4 {
		t.Errorf("MaxConcurrent = %d, want 4", cfg.Collector.MaxConcurrent)
	}
	if cfg.DBConfig.Driver != "postgres" {
		t.Errorf("Driver = %q, want postgres", cfg.DBConfig.Driver)
	}
	if len(cfg.Devices) != 1 {
		t.Fatalf("len(Devices) = %d, want 1", len(cfg.Devices))
	}
	d := cfg.Devices[0]
	if d.ID != "dev-1" || d.Name != "Device One" || d.Address != "10.0.0.1:50051" || d.Protocol != ProtocolGRPC {
		t.Errorf("unexpected device config: %+v", d)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	path := writeTemp(t, "server:\n  grpc_addr: \":9999\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.GRPCAddr != ":9999" {
		t.Errorf("GRPCAddr = %q, want :9999", cfg.Server.GRPCAddr)
	}
	if cfg.Collector.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want default 30s", cfg.Collector.Interval)
	}
	if cfg.Server.RESTAddr != ":8080" {
		t.Errorf("RESTAddr = %q, want default :8080", cfg.Server.RESTAddr)
	}
}

func TestLoad_Error(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTemp(t, "server: [unclosed")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestDeviceProtocolToProto(t *testing.T) {
	cases := []struct {
		in   DeviceProtocol
		want pb.DeviceProtocol
	}{
		{ProtocolGRPC, pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC},
		{ProtocolREST, pb.DeviceProtocol_DEVICE_PROTOCOL_REST},
		{"bogus", pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED},
		{"", pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := c.in.ToProto(); got != c.want {
			t.Errorf("ToProto(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
