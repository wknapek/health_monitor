package service

import (
	"context"
	"fmt"
	"log/slog"

	"ubi/internal/model"
	"ubi/internal/store"

	"google.golang.org/protobuf/types/known/timestamppb"
	pb "ubi/proto/monitor/v1"
)

type Service struct {
	store  store.Storage
	logger *slog.Logger
}

func New(st store.Storage, logger *slog.Logger) *Service {
	return &Service{store: st, logger: logger}
}

func (s *Service) ListDevices(ctx context.Context) []*pb.Device {
	devices := s.store.ListDevices()
	out := make([]*pb.Device, 0, len(devices))
	for _, d := range devices {
		out = append(out, toProtoDevice(d))
	}
	return out
}

func (s *Service) GetDevice(ctx context.Context, id string) (*pb.Device, error) {
	d, ok := s.store.GetDevice(id)
	if !ok {
		return nil, fmt.Errorf("device %q not found", id)
	}
	return toProtoDevice(d), nil
}

func (s *Service) GetDiagnostics(ctx context.Context, id string) (*pb.Diagnostics, error) {
	diag, ok := s.store.GetDiagnostics(id)
	if !ok {
		return nil, fmt.Errorf("no diagnostics for device %q", id)
	}
	return toProtoDiagnostics(diag), nil
}

func (s *Service) AddDevice(ctx context.Context, id, name, address string, protocol pb.DeviceProtocol) (*pb.Device, error) {
	if id == "" {
		s.logger.Warn("add device failed: id required")
		return nil, fmt.Errorf("device id is required")
	}
	if address == "" {
		s.logger.Warn("add device failed: address required", "id", id)
		return nil, fmt.Errorf("device address is required")
	}
	if protocol != pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC &&
		protocol != pb.DeviceProtocol_DEVICE_PROTOCOL_REST {
		s.logger.Warn("add device failed: invalid protocol", "id", id, "protocol", protocol.String())
		return nil, fmt.Errorf("invalid protocol: %v", protocol)
	}
	if _, exists := s.store.GetDevice(id); exists {
		s.logger.Warn("add device failed: already exists", "id", id)
		return nil, fmt.Errorf("device %q already exists", id)
	}

	d := &model.Device{
		ID:       id,
		Name:     name,
		Address:  address,
		Protocol: protocol,
		Status:   pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED,
	}
	s.store.AddDevice(d)
	s.logger.Info("device registered", "id", id, "name", name, "address", address, "protocol", protocol.String())
	return toProtoDevice(d), nil
}

func (s *Service) RemoveDevice(ctx context.Context, id string) error {
	if !s.store.RemoveDevice(id) {
		s.logger.Warn("remove device failed: not found", "id", id)
		return fmt.Errorf("device %q not found", id)
	}
	s.logger.Info("device removed", "id", id)
	return nil
}

func toProtoDevice(d *model.Device) *pb.Device {
	pd := &pb.Device{
		Id:           d.ID,
		Name:         d.Name,
		Address:      d.Address,
		Protocol:     d.Protocol,
		Status:       d.Status,
		Capabilities: d.Capabilities,
	}
	if !d.LastChecked.IsZero() {
		pd.LastChecked = timestamppb.New(d.LastChecked)
	}
	return pd
}

func toProtoDiagnostics(d *model.Diagnostics) *pb.Diagnostics {
	diag := &pb.Diagnostics{
		DeviceId:        d.DeviceID,
		HardwareVersion: d.HardwareVersion,
		SoftwareVersion: d.SoftwareVersion,
		FirmwareVersion: d.FirmwareVersion,
		Status:          d.Status,
		Checksum:        d.Checksum,
	}
	if !d.CollectedAt.IsZero() {
		diag.CollectedAt = timestamppb.New(d.CollectedAt)
	}
	return diag
}
