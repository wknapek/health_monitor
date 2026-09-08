package store

import (
	"testing"
	"time"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

func newDevice(id string) *model.Device {
	return &model.Device{
		ID:       id,
		Name:     "Device " + id,
		Address:  "10.0.0.1",
		Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST,
	}
}

func TestAddAndGetDevice(t *testing.T) {
	s := New()
	d := newDevice("dev-1")
	s.AddDevice(d)

	got, ok := s.GetDevice("dev-1")
	if !ok {
		t.Fatal("expected device to exist")
	}
	if got.ID != "dev-1" {
		t.Errorf("ID = %q, want dev-1", got.ID)
	}
}

func TestGetDevice_Missing(t *testing.T) {
	s := New()
	if _, ok := s.GetDevice("nope"); ok {
		t.Fatal("expected no device for missing id")
	}
}

func TestAddOverwrites(t *testing.T) {
	s := New()
	s.AddDevice(newDevice("dev-1"))
	s.AddDevice(&model.Device{ID: "dev-1", Name: "Updated", Address: "x"})

	got, _ := s.GetDevice("dev-1")
	if got.Name != "Updated" {
		t.Errorf("Name = %q, want Updated", got.Name)
	}
}

func TestRemoveDevice(t *testing.T) {
	s := New()
	s.AddDevice(newDevice("dev-1"))
	s.SetDiagnostics("dev-1", &model.Diagnostics{DeviceID: "dev-1"})

	if !s.RemoveDevice("dev-1") {
		t.Fatal("RemoveDevice should return true for existing device")
	}
	if _, ok := s.GetDevice("dev-1"); ok {
		t.Error("device still present after removal")
	}
	if _, ok := s.GetDiagnostics("dev-1"); ok {
		t.Error("diagnostics still present after removal")
	}
	if s.RemoveDevice("dev-1") {
		t.Error("RemoveDevice should return false for missing device")
	}
}

func TestListDevices(t *testing.T) {
	s := New()
	s.AddDevice(newDevice("dev-1"))
	s.AddDevice(newDevice("dev-2"))

	devices := s.ListDevices()
	ids := map[string]bool{}
	for _, d := range devices {
		ids[d.ID] = true
	}
	if !ids["dev-1"] || !ids["dev-2"] {
		t.Errorf("ListDevices missing expected ids, got %v", ids)
	}
	if len(devices) != 2 {
		t.Errorf("len(ListDevices) = %d, want 2", len(devices))
	}
}

func TestListDevices_Empty(t *testing.T) {
	s := New()
	if devices := s.ListDevices(); len(devices) != 0 {
		t.Errorf("len(ListDevices) = %d, want 0", len(devices))
	}
}

func TestUpdateDeviceStatus(t *testing.T) {
	s := New()
	d := newDevice("dev-1")
	s.AddDevice(d)

	before := time.Now()
	s.UpdateDeviceStatus("dev-1", pb.DeviceStatus_DEVICE_STATUS_UP, []string{"hw", "sw"})

	got, _ := s.GetDevice("dev-1")
	if got.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("Status = %v, want UP", got.Status)
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities len = %d, want 2", len(got.Capabilities))
	}
	if got.LastChecked.Before(before) {
		t.Error("LastChecked should be updated")
	}
}

func TestUpdateDeviceStatus_MissingNoop(t *testing.T) {
	s := New()
	s.UpdateDeviceStatus("ghost", pb.DeviceStatus_DEVICE_STATUS_UP, nil)
	if _, ok := s.GetDevice("ghost"); ok {
		t.Error("device should not be created")
	}
}

func TestSetGetDiagnostics(t *testing.T) {
	s := New()
	diag := &model.Diagnostics{
		DeviceID:        "dev-1",
		HardwareVersion: "1.0",
		Status:          pb.DeviceStatus_DEVICE_STATUS_UP,
	}
	s.SetDiagnostics("dev-1", diag)

	got, ok := s.GetDiagnostics("dev-1")
	if !ok {
		t.Fatal("expected diagnostics to exist")
	}
	if got.HardwareVersion != "1.0" {
		t.Errorf("HardwareVersion = %q, want 1.0", got.HardwareVersion)
	}
}

func TestGetDiagnostics_Missing(t *testing.T) {
	s := New()
	if _, ok := s.GetDiagnostics("dev-1"); ok {
		t.Fatal("should not have diagnostics for missing device")
	}
}

// TestStoreImplementsStorage is a compile-time interface check.
func TestStoreImplementsStorage(t *testing.T) {
	var _ Storage = New()
}
