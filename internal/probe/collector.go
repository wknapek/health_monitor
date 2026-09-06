package probe

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"ubi/internal/config"
	"ubi/internal/model"
	"ubi/internal/store"
	pb "ubi/proto/monitor/v1"
)

type Collector struct {
	store     *store.Store
	grpcProbe *grpcProbe
	restProbe *restProbe
	interval  time.Duration
	maxConc   int
	logger    *slog.Logger
}

func NewCollector(st *store.Store, cfg config.CollectorConfig, logger *slog.Logger) *Collector {
	return &Collector{
		store:     st,
		grpcProbe: newGRPCProbe(cfg.GRPCTimeout, logger),
		restProbe: newRESTProbe(cfg.RESTTimeout, logger),
		interval:  cfg.Interval,
		maxConc:   cfg.MaxConcurrent,
		logger:    logger,
	}
}

// Run blocks, probing all devices on an interval until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	// Probe immediately, then on the ticker.
	c.ProbeAll(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.ProbeAll(ctx)
		}
	}
}

// ProbeAll probes every device concurrently, respecting maxConcurrent.
func (c *Collector) ProbeAll(ctx context.Context) {
	start := time.Now()
	devices := c.store.ListDevices()
	if len(devices) == 0 {
		return
	}
	c.logger.Info("probe cycle started", "devices", len(devices))

	sem := make(chan struct{}, c.maxConc)
	var wg sync.WaitGroup

	for _, d := range devices {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(dev *model.Device) {
			defer wg.Done()
			defer func() { <-sem }() // release
			c.probeOne(ctx, dev)
		}(d)
	}

	wg.Wait()
	c.logger.Info("probe cycle completed",
		"devices", len(devices),
		"duration_ms", time.Since(start).Milliseconds())
}

// probeOne probes a single device and persists the result.
func (c *Collector) probeOne(ctx context.Context, d *model.Device) {
	start := time.Now()
	var res *model.ProbeResult

	switch d.Protocol {
	case pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC:
		res = c.grpcProbe.Probe(ctx, d.Address, d.Capabilities)
	case pb.DeviceProtocol_DEVICE_PROTOCOL_REST:
		res = c.restProbe.Probe(ctx, d.Address)
	default:
		res = &model.ProbeResult{Status: pb.DeviceStatus_DEVICE_STATUS_DEGRADED}
	}

	prevStatus := d.Status
	c.store.UpdateDeviceStatus(d.ID, res.Status, res.Capabilities)

	logAttrs := []any{
		"id", d.ID,
		"address", d.Address,
		"status", res.Status.String(),
		"duration_ms", time.Since(start).Milliseconds(),
	}

	if prevStatus != res.Status {
		c.logger.Warn("device status changed",
			append(logAttrs, "prev_status", prevStatus.String())...)
	}

	if res.Diagnostics != nil {
		res.Diagnostics.DeviceID = d.ID
		c.store.SetDiagnostics(d.ID, res.Diagnostics)
		c.logger.Info("probed device", append(logAttrs, "diagnostics", true)...)
	} else {
		c.logger.Info("probed device (no diagnostics)", append(logAttrs, "diagnostics", false)...)
	}
}
