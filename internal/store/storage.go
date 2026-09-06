package store

import (
	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

// Storage defines the persistence contract for devices and their diagnostics.
// Any backend — in-memory, SQL (GORM/MySQL/SQLite), etc. — can implement this
// interface, and the rest of the application depends only on this abstraction.
type Storage interface {
	// AddDevice registers a device (or updates it in place).
	AddDevice(d *model.Device)

	// RemoveDevice deletes a device and its diagnostics by id, reporting
	// whether a matching device existed.
	RemoveDevice(id string) bool

	// GetDevice returns a device by id and whether it exists.
	GetDevice(id string) (*model.Device, bool)

	// ListDevices returns all tracked devices.
	ListDevices() []*model.Device

	// UpdateDeviceStatus updates a device's status, last-checked time, and
	// capabilities after a probe. It is a no-op if the device does not exist.
	UpdateDeviceStatus(id string, status pb.DeviceStatus, caps []string)

	// SetDiagnostics stores the latest diagnostics for a device.
	SetDiagnostics(id string, diag *model.Diagnostics)

	// GetDiagnostics returns the latest diagnostics for a device, reporting
	// whether any exist.
	GetDiagnostics(id string) (*model.Diagnostics, bool)
}
