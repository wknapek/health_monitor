package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ubi/internal/service"
	pb "ubi/proto/monitor/v1"
)

type RESTServer struct {
	svc    *service.Service
	mux    *http.ServeMux
	logger *slog.Logger
}

func NewRESTServer(svc *service.Service, logger *slog.Logger) *RESTServer {
	s := &RESTServer{
		svc:    svc,
		mux:    http.NewServeMux(),
		logger: logger,
	}
	s.routes()
	return s
}

func (s *RESTServer) routes() {
	s.mux.HandleFunc("GET /api/v1/devices", s.handleListDevices)
	s.mux.HandleFunc("GET /api/v1/devices/{id}", s.handleGetDevice)
	s.mux.HandleFunc("GET /api/v1/devices/{id}/diagnostics", s.handleGetDiagnostics)
	s.mux.HandleFunc("POST /api/v1/devices", s.handleAddDevice)
	s.mux.HandleFunc("DELETE /api/v1/devices/{id}", s.handleRemoveDevice)
}

func (s *RESTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(sw, r)
	s.logger.Info("request",
		"method", r.Method,
		"path", r.URL.Path,
		"status", sw.status,
		"duration_ms", time.Since(start).Milliseconds(),
		"remote", r.RemoteAddr,
	)
}

func (s *RESTServer) handleListDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": s.svc.ListDevices(r.Context()),
	})
}

func (s *RESTServer) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.svc.GetDevice(r.Context(), id)
	if err != nil {
		s.logger.Warn("device not found", "id", id, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": d})
}

func (s *RESTServer) handleGetDiagnostics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := s.svc.GetDiagnostics(r.Context(), id)
	if err != nil {
		s.logger.Warn("diagnostics not found", "id", id, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": d})
}

type addDeviceRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Protocol string `json:"protocol"`
}

func (s *RESTServer) handleAddDevice(w http.ResponseWriter, r *http.Request) {
	var req addDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Warn("invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	protocol, err := parseProtocol(req.Protocol)
	if err != nil {
		s.logger.Warn("invalid protocol", "protocol", req.Protocol, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	d, err := s.svc.AddDevice(r.Context(), req.ID, req.Name, req.Address, protocol)
	if err != nil {
		s.logger.Warn("failed to add device", "id", req.ID, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Info("device added", "id", req.ID, "name", req.Name, "address", req.Address, "protocol", req.Protocol)
	writeJSON(w, http.StatusCreated, map[string]any{"device": d})
}

func (s *RESTServer) handleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.svc.RemoveDevice(r.Context(), id); err != nil {
		s.logger.Warn("failed to remove device", "id", id, "error", err)
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.logger.Info("device removed", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func parseProtocol(s string) (pb.DeviceProtocol, error) {
	switch strings.ToLower(s) {
	case "grpc":
		return pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC, nil
	case "rest", "http":
		return pb.DeviceProtocol_DEVICE_PROTOCOL_REST, nil
	default:
		return pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED,
			&protocolError{s}
	}
}

type protocolError struct{ v string }

func (e *protocolError) Error() string {
	return "invalid protocol: " + e.v + " (expected grpc or rest)"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
