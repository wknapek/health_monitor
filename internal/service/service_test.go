package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"ubi/internal/model"
	"ubi/internal/store"

	pb "ubi/proto/monitor/v1"
)

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st := store.New()
	logger := slog.New(slog.DiscardHandler)
	return New(st, logger), st
}

func TestAddDevice_Valid(t *testing.T) {
	svc, st := newTestService(t)
	d, err := svc.AddDevice(context.Background(), "dev-1", "One", "10.0.0.1", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC)
	if err != nil {
		t.Fatalf("AddDevice() error = %v", err)
	}
	if d.Id != "dev-1" || d.Name != "One" || d.Address != "10.0.0.1" {
		t.Errorf("unexpected device: %+v", d)
	}
	if got, ok := st.GetDevice("dev-1"); !ok || got.Name != "One" {
		t.Errorf("device not persisted correctly: %+v", got)
	}
}

func TestAddDevice_Validation(t *testing.T) {
	cases := []struct {
		name  string
		id    string
		addr  string
		proto pb.DeviceProtocol
	}{
		{"missing id", "", "10.0.0.1", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC},
		{"missing address", "x", "", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC},
		{"invalid protocol", "x", "10.0.0.1", pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, _ := newTestService(t)
			if _, err := svc.AddDevice(context.Background(), c.id, "n", c.addr, c.proto); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestAddDevice_Duplicate(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.AddDevice(ctx, "dev-1", "One", "10.0.0.1", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddDevice(ctx, "dev-1", "Two", "10.0.0.2", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestGetDevice(t *testing.T) {
	svc, st := newTestService(t)
	st.AddDevice(&model.Device{
		ID:          "dev-1",
		Name:        "One",
		Address:     "10.0.0.1",
		Protocol:    pb.DeviceProtocol_DEVICE_PROTOCOL_REST,
		Status:      pb.DeviceStatus_DEVICE_STATUS_UP,
		LastChecked: time.Unix(1700000000, 0),
	})

	d, err := svc.GetDevice(context.Background(), "dev-1")
	if err != nil {
		t.Fatalf("GetDevice() error = %v", err)
	}
	if d.Id != "dev-1" || d.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("unexpected device: %+v", d)
	}
	if d.LastChecked == nil {
		t.Error("LastChecked should be populated")
	}
}

func TestGetDevice_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.GetDevice(context.Background(), "missing"); err == nil {
		t.Fatal("expected not found error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want contains 'not found'", err)
	}
}

func TestGetDiagnostics(t *testing.T) {
	svc, st := newTestService(t)
	st.AddDevice(&model.Device{ID: "dev-1", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})
	st.SetDiagnostics("dev-1", &model.Diagnostics{
		DeviceID:        "dev-1",
		HardwareVersion: "hw-2",
		SoftwareVersion: "sw-3",
		FirmwareVersion: "fw-4",
		Status:          pb.DeviceStatus_DEVICE_STATUS_DEGRADED,
		Checksum:        "abc123",
	})

	diag, err := svc.GetDiagnostics(context.Background(), "dev-1")
	if err != nil {
		t.Fatalf("GetDiagnostics() error = %v", err)
	}
	if diag.HardwareVersion != "hw-2" || diag.SoftwareVersion != "sw-3" || diag.FirmwareVersion != "fw-4" || diag.Checksum != "abc123" {
		t.Errorf("unexpected diagnostics: %+v", diag)
	}
	if diag.Status != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("Status = %v, want DEGRADED", diag.Status)
	}
}

func TestGetDiagnostics_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.GetDiagnostics(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing diagnostics")
	}
}

func TestListDevices(t *testing.T) {
	svc, st := newTestService(t)
	st.AddDevice(&model.Device{ID: "dev-1", Address: "a", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC})
	st.AddDevice(&model.Device{ID: "dev-2", Address: "b", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})

	devices := svc.ListDevices(context.Background())
	if len(devices) != 2 {
		t.Fatalf("len(ListDevices) = %d, want 2", len(devices))
	}
}

func TestRemoveDevice(t *testing.T) {
	svc, st := newTestService(t)
	st.AddDevice(&model.Device{ID: "dev-1", Address: "a", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC})
	if err := svc.RemoveDevice(context.Background(), "dev-1"); err != nil {
		t.Fatalf("RemoveDevice() error = %v", err)
	}
	if _, ok := st.GetDevice("dev-1"); ok {
		t.Error("device still exists after removal")
	}
}

func TestRemoveDevice_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	if err := svc.RemoveDevice(context.Background(), "missing"); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestToProtoDevice_NoLastChecked(t *testing.T) {
	d := toProtoDevice(&model.Device{ID: "x", Name: "n"})
	if d.LastChecked != nil {
		t.Error("LastChecked should be nil when zero")
	}
}
