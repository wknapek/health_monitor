package model

import (
	"time"

	"gorm.io/gorm"

	pb "ubi/proto/monitor/v1"
)

// Device represents a tracked network device. It maps to the "devices" table.
type Device struct {
	gorm.Model
	ID           string            `gorm:"primaryKey;size:255;not null;index"`
	Name         string            `gorm:"size:255;not null"`
	Address      string            `gorm:"size:255;not null"`
	Protocol     pb.DeviceProtocol `gorm:"not null;index"`           // stored as int (proto enum)
	Status       pb.DeviceStatus   `gorm:"not null;default:0;index"` // stored as int (proto enum)
	LastChecked  time.Time
	Capabilities []string     `gorm:"serializer:json"`
	Diagnostics  *Diagnostics `gorm:"foreignKey:DeviceID"`
}

// Diagnostics holds the latest collected health/version data for a device. It
// maps to the "diagnostics" table, linked one-to-one to a Device via DeviceID,
// which is the primary key.
type Diagnostics struct {
	DeviceID        string          `gorm:"primaryKey;size:255;not null"`
	HardwareVersion string          `gorm:"size:255"`
	SoftwareVersion string          `gorm:"size:255"`
	FirmwareVersion string          `gorm:"size:255"`
	Status          pb.DeviceStatus `gorm:"not null;default:0"`
	Checksum        string          `gorm:"size:255"`
	CollectedAt     time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ProbeResult is a transient value object produced by probing a device. It is
// not persisted directly.
type ProbeResult struct {
	Status       pb.DeviceStatus
	Capabilities []string
	Diagnostics  *Diagnostics
}
