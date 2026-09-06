package model

import (
	"time"

	pb "ubi/proto/monitor/v1"
)

type Device struct {
	ID           string
	Name         string
	Address      string
	Protocol     pb.DeviceProtocol
	Status       pb.DeviceStatus
	LastChecked  time.Time
	Capabilities []string
}

type Diagnostics struct {
	DeviceID        string
	HardwareVersion string
	SoftwareVersion string
	FirmwareVersion string
	Status          pb.DeviceStatus
	Checksum        string
	CollectedAt     time.Time
}

type ProbeResult struct {
	Status       pb.DeviceStatus
	Capabilities []string
	Diagnostics  *Diagnostics
}
