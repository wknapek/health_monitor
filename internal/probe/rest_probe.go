package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ubi/internal/model"
	pb "ubi/proto/monitor/v1"
)

var maxRetries = 5

type healthResponse struct {
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

type restProbe struct {
	timeout time.Duration
	client  *http.Client
	logger  *slog.Logger
}

func newRESTProbe(timeout time.Duration, logger *slog.Logger) *restProbe {
	return &restProbe{
		timeout: timeout,
		logger:  logger,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *restProbe) Probe(ctx context.Context, baseURL string) *model.ProbeResult {
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	hr := &healthResponse{}
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Liveness + capability discovery via /health.

		hr, err = p.getHealth(probeCtx, baseURL)
		if err == nil {
			break
		}
		p.logger.Debug("failed to check health", "address", baseURL, "err", err)
		p.logger.Debug("retrying health check", "address", baseURL, "attempt", attempt+1)
	}
	if err != nil {
		p.logger.Warn("REST device unhealthy", "address", baseURL, "error", err)
		return &model.ProbeResult{Status: pb.DeviceStatus_DEVICE_STATUS_DOWN}
	}

	p.logger.Debug("REST health check ok", "address", baseURL, "status", hr.Status)

	if !hr.Serving() {
		return &model.ProbeResult{Status: hr.StatusEnum()}
	}

	caps := hr.NormalizedCaps()
	r := &model.ProbeResult{
		Status:       pb.DeviceStatus_DEVICE_STATUS_UP,
		Capabilities: caps,
	}
	p.logger.Debug("REST capabilities", "address", baseURL, "capabilities", caps)

	// Capability-driven diagnostics.
	if diag := p.collectDiagnostics(probeCtx, baseURL, caps); diag != nil {
		r.Diagnostics = diag
		p.logger.Debug("REST diagnostics collected", "address", baseURL,
			"hw", diag.HardwareVersion, "sw", diag.SoftwareVersion, "fw", diag.FirmwareVersion)
		if diag.Status != pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED {
			r.Status = diag.Status
		}
	} else {
		p.logger.Debug("no REST diagnostics collected", "address", baseURL, "capabilities", caps)
	}

	return r
}

func (p *restProbe) getHealth(ctx context.Context, baseURL string) (*healthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health returned status %d", resp.StatusCode)
	}

	var hr healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		// Some devices return a bare "ok"/"up" body; decode best-effort.
		p.logger.Debug("non-standard health body", "address", baseURL, "error", err)
		hr = healthResponse{}
	}
	return &hr, nil
}

func (p *restProbe) collectDiagnostics(ctx context.Context, baseURL string, caps []string) *model.Diagnostics {
	if len(caps) == 0 {
		return nil
	}

	// Devices that support it expose GET /diagnostics returning all fields.
	if diag := p.get(ctx, baseURL+"/diagnostics"); diag != nil {
		return diag
	}

	// Otherwise fetch per-capability endpoints.
	diag := &model.Diagnostics{CollectedAt: time.Now()}
	for _, cap := range caps {
		d := p.get(ctx, baseURL+"/diagnostics/"+cap)
		if d == nil {
			continue
		}
		mergeDiagnostics(diag, d)
	}
	return diag
}

// get fetches a single diagnostic payload; returns nil on any failure.
func (p *restProbe) get(ctx context.Context, url string) *model.Diagnostics {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		p.logger.Debug("diagnostics request build failed", "url", url, "error", err)
		return nil
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Debug("diagnostics request failed", "url", url, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.logger.Debug("diagnostics returned non-200", "url", url, "status", resp.StatusCode)
		return nil
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		p.logger.Debug("diagnostics decode failed", "url", url, "error", err)
		return nil
	}

	d := &model.Diagnostics{CollectedAt: time.Now()}
	mergeJSONDiagnostics(d, payload)
	return d
}

func mergeJSONDiagnostics(d *model.Diagnostics, m map[string]any) {
	for key, val := range m {
		s, _ := val.(string)
		switch strings.ToLower(key) {
		case "hw", "hw_version", "hardware", "hardware_version":
			d.HardwareVersion = s
		case "sw", "sw_version", "software", "software_version":
			d.SoftwareVersion = s
		case "fw", "fw_version", "firmware", "firmware_version":
			d.FirmwareVersion = s
		case "checksum":
			d.Checksum = s
		case "status", "state":
			if st := parseStatus(s); st != pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED {
				d.Status = st
			}
		}
	}
}

func mergeDiagnostics(dst, src *model.Diagnostics) {
	if src == nil {
		return
	}
	if src.HardwareVersion != "" {
		dst.HardwareVersion = src.HardwareVersion
	}
	if src.SoftwareVersion != "" {
		dst.SoftwareVersion = src.SoftwareVersion
	}
	if src.FirmwareVersion != "" {
		dst.FirmwareVersion = src.FirmwareVersion
	}
	if src.Checksum != "" {
		dst.Checksum = src.Checksum
	}
	if src.Status != pb.DeviceStatus_DEVICE_STATUS_UNSPECIFIED {
		dst.Status = src.Status
	}
}

func normalizeStringStatus(s string) string {
	switch s {
	case "up", "healthy", "ok", "serving":
		return "up"
	case "down", "unreachable", "not_serving":
		return "down"
	case "degraded", "unknown", "warning":
		return "degraded"
	}
	return s
}

func (hr *healthResponse) Serving() bool {
	return normalizeStringStatus(hr.Status) == "up"
}

func (hr *healthResponse) StatusEnum() pb.DeviceStatus {
	switch normalizeStringStatus(hr.Status) {
	case "up":
		return pb.DeviceStatus_DEVICE_STATUS_UP
	case "down":
		return pb.DeviceStatus_DEVICE_STATUS_DOWN
	default:
		return pb.DeviceStatus_DEVICE_STATUS_DEGRADED
	}
}

func (hr *healthResponse) NormalizedCaps() []string {
	return hr.Capabilities
}
