package config

import (
	"os"
	"time"

	pb "ubi/proto/monitor/v1"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Collector CollectorConfig `yaml:"collector"`
	Devices   []DeviceConfig  `yaml:"devices"`
}

type ServerConfig struct {
	GRPCAddr string `yaml:"grpc_addr"`
	RESTAddr string `yaml:"rest_addr"`
}

type CollectorConfig struct {
	Interval      time.Duration `yaml:"interval"`
	GRPCTimeout   time.Duration `yaml:"grpc_timeout"`
	RESTTimeout   time.Duration `yaml:"rest_timeout"`
	MaxConcurrent int           `yaml:"max_concurrent"`
}

type DeviceConfig struct {
	ID       string         `yaml:"id"`
	Name     string         `yaml:"name"`
	Address  string         `yaml:"address"`
	Protocol DeviceProtocol `yaml:"protocol"`
}

type DeviceProtocol string

const (
	ProtocolGRPC DeviceProtocol = "grpc"
	ProtocolREST DeviceProtocol = "rest"
)

func (dp DeviceProtocol) ToProto() pb.DeviceProtocol {
	switch dp {
	case ProtocolGRPC:
		return pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC
	case ProtocolREST:
		return pb.DeviceProtocol_DEVICE_PROTOCOL_REST
	default:
		return pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			GRPCAddr: ":9090",
			RESTAddr: ":8080",
		},
		Collector: CollectorConfig{
			Interval:      30 * time.Second,
			GRPCTimeout:   5 * time.Second,
			RESTTimeout:   5 * time.Second,
			MaxConcurrent: 10,
		},
	}
}
