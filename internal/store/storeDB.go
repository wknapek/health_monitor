package store

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

// StoreDB is a GORM-backed implementation of Storage, usable with any
// database driver supported by GORM (MySQL, SQLite, Postgres, etc.).
type StoreDB struct {
	db *gorm.DB
}

var _ Storage = (*StoreDB)(nil)

// NewStoreDB returns a DB-backed store, auto-migrating the schema.
func NewStoreDB(db *gorm.DB) *StoreDB {
	return &StoreDB{db: db}
}

// AutoMigrate creates/updates the schema for the store's models.
func (s *StoreDB) AutoMigrate() error {
	return s.db.AutoMigrate(&model.Device{}, &model.Diagnostics{})
}

func (s *StoreDB) AddDevice(d *model.Device) {
	s.db.Save(d)
}

func (s *StoreDB) RemoveDevice(id string) bool {
	tx := s.db.Where("device_id = ?", id).Delete(&model.Diagnostics{})
	if err := tx.Error; err != nil {
		return false
	}
	res := s.db.Delete(&model.Device{}, "id = ?", id)
	return res.Error == nil && res.RowsAffected > 0
}

func (s *StoreDB) GetDevice(id string) (*model.Device, bool) {
	var d model.Device
	if err := s.db.First(&d, "id = ?", id).Error; err != nil {
		return nil, false
	}
	return &d, true
}

func (s *StoreDB) ListDevices() []*model.Device {
	var devices []*model.Device
	s.db.Find(&devices)
	return devices
}

func (s *StoreDB) UpdateDeviceStatus(id string, status pb.DeviceStatus, caps []string) {
	s.db.Model(&model.Device{}).
		Where("id = ?", id).
		Select("status", "last_checked", "capabilities").
		Updates(&model.Device{
			Status:       status,
			LastChecked:  time.Now(),
			Capabilities: caps,
		})
}

func (s *StoreDB) SetDiagnostics(id string, diag *model.Diagnostics) {
	diag.DeviceID = id
	s.db.Save(diag)
}

func (s *StoreDB) GetDiagnostics(id string) (*model.Diagnostics, bool) {
	var d model.Diagnostics
	if err := s.db.First(&d, "device_id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false
		}
		return nil, false
	}
	return &d, true
}
