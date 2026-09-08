package store

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

func newDBStore(t *testing.T) *StoreDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	s := NewStoreDB(db)
	if err := s.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return s
}

func TestStoreDB_AddGetDevice(t *testing.T) {
	s := newDBStore(t)
	d := &model.Device{
		ID:       "dev-1",
		Name:     "One",
		Address:  "10.0.0.1",
		Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC,
	}
	s.AddDevice(d)

	got, ok := s.GetDevice("dev-1")
	if !ok {
		t.Fatal("expected device to exist")
	}
	if got.Name != "One" || got.Protocol != pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC {
		t.Errorf("unexpected device: %+v", got)
	}
}

func TestStoreDB_GetDevice_Missing(t *testing.T) {
	s := newDBStore(t)
	if _, ok := s.GetDevice("missing"); ok {
		t.Fatal("should not find missing device")
	}
}

func TestStoreDB_AddOverwrites(t *testing.T) {
	s := newDBStore(t)
	s.AddDevice(&model.Device{ID: "dev-1", Name: "Old", Address: "a"})
	s.AddDevice(&model.Device{ID: "dev-1", Name: "New", Address: "b"})

	got, _ := s.GetDevice("dev-1")
	if got.Name != "New" {
		t.Errorf("Name = %q, want New", got.Name)
	}
}

func TestStoreDB_ListDevices(t *testing.T) {
	s := newDBStore(t)
	s.AddDevice(&model.Device{ID: "dev-1", Name: "One", Address: "a"})
	s.AddDevice(&model.Device{ID: "dev-2", Name: "Two", Address: "b"})

	devices := s.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}
	ids := map[string]bool{}
	for _, d := range devices {
		ids[d.ID] = true
	}
	if !ids["dev-1"] || !ids["dev-2"] {
		t.Errorf("missing ids: %v", ids)
	}
}

func TestStoreDB_RemoveDevice(t *testing.T) {
	s := newDBStore(t)
	s.AddDevice(&model.Device{ID: "dev-1", Name: "One", Address: "a"})
	s.SetDiagnostics("dev-1", &model.Diagnostics{DeviceID: "dev-1"})

	if !s.RemoveDevice("dev-1") {
		t.Fatal("RemoveDevice should return true for existing device")
	}
	if _, ok := s.GetDevice("dev-1"); ok {
		t.Error("device still exists")
	}
	if _, ok := s.GetDiagnostics("dev-1"); ok {
		t.Error("diagnostics still exist")
	}
	if s.RemoveDevice("dev-1") {
		t.Error("RemoveDevice should return false for missing device")
	}
}

func TestStoreDB_UpdateDeviceStatus(t *testing.T) {
	s := newDBStore(t)
	s.AddDevice(&model.Device{ID: "dev-1", Name: "One", Address: "a"})
	before := time.Now()
	s.UpdateDeviceStatus("dev-1", pb.DeviceStatus_DEVICE_STATUS_UP, []string{"hw", "sw"})

	got, ok := s.GetDevice("dev-1")
	if !ok {
		t.Fatal("device missing")
	}
	if got.Status != pb.DeviceStatus_DEVICE_STATUS_UP {
		t.Errorf("Status = %v, want UP", got.Status)
	}
	if len(got.Capabilities) != 2 {
		t.Errorf("Capabilities = %v, want 2 items", got.Capabilities)
	}
	if got.LastChecked.Before(before) {
		t.Error("LastChecked should be updated")
	}
}

func TestStoreDB_UpdateDeviceStatus_MissingNoop(t *testing.T) {
	s := newDBStore(t)
	s.UpdateDeviceStatus("ghost", pb.DeviceStatus_DEVICE_STATUS_UP, nil)
	if _, ok := s.GetDevice("ghost"); ok {
		t.Error("device should not be created")
	}
}

func TestStoreDB_SetGetDiagnostics(t *testing.T) {
	s := newDBStore(t)
	s.AddDevice(&model.Device{ID: "dev-1", Name: "One", Address: "a"})
	diag := &model.Diagnostics{
		DeviceID:        "dev-1",
		HardwareVersion: "HW",
		SoftwareVersion: "SW",
		FirmwareVersion: "FW",
		Checksum:        "abc",
		Status:          pb.DeviceStatus_DEVICE_STATUS_DEGRADED,
		CollectedAt:     time.Now(),
	}
	s.SetDiagnostics("dev-1", diag)

	got, ok := s.GetDiagnostics("dev-1")
	if !ok {
		t.Fatal("expected diagnostics")
	}
	if got.HardwareVersion != "HW" || got.SoftwareVersion != "SW" || got.Checksum != "abc" {
		t.Errorf("unexpected diagnostics: %+v", got)
	}
	if got.Status != pb.DeviceStatus_DEVICE_STATUS_DEGRADED {
		t.Errorf("Status = %v, want DEGRADED", got.Status)
	}
}

func TestStoreDB_GetDiagnostics_Missing(t *testing.T) {
	s := newDBStore(t)
	if _, ok := s.GetDiagnostics("ghost"); ok {
		t.Fatal("should not have diagnostics for missing device")
	}
}

// TestStoreDBImplementsStorage is a compile-time interface check.
func TestStoreDBImplementsStorage(t *testing.T) {
	var _ Storage = (*StoreDB)(nil)
}
