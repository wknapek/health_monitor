package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ubi/internal/model"
	"ubi/internal/service"
	"ubi/internal/store"
	pb "ubi/proto/monitor/v1"
)

func newRESTTestServer(t *testing.T) (*RESTServer, *store.Store) {
	t.Helper()
	st := store.New()
	logger := slog.New(slog.DiscardHandler)
	svc := service.New(st, logger)
	return NewRESTServer(svc, logger), st
}

func doRequest(t *testing.T, rs *RESTServer, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	return rec
}

func TestREST_ListDevices(t *testing.T) {
	rs, st := newRESTTestServer(t)
	st.AddDevice(&model.Device{ID: "d1", Name: "One", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})
	st.AddDevice(&model.Device{ID: "d2", Name: "Two", Address: "10.0.0.2", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC})

	rec := doRequest(t, rs, http.MethodGet, "/api/v1/devices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Devices []*pb.Device `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Devices) != 2 {
		t.Errorf("len(devices) = %d, want 2", len(resp.Devices))
	}
}

func TestREST_GetDevice(t *testing.T) {
	rs, st := newRESTTestServer(t)
	st.AddDevice(&model.Device{ID: "d1", Name: "One", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})

	rec := doRequest(t, rs, http.MethodGet, "/api/v1/devices/d1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Device *pb.Device `json:"device"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Device == nil || resp.Device.Id != "d1" {
		t.Errorf("unexpected device: %+v", resp.Device)
	}
}

func TestREST_GetDevice_NotFound(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodGet, "/api/v1/devices/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestREST_GetDiagnostics(t *testing.T) {
	rs, st := newRESTTestServer(t)
	st.AddDevice(&model.Device{ID: "d1", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})
	st.SetDiagnostics("d1", &model.Diagnostics{
		DeviceID:        "d1",
		HardwareVersion: "HW",
		SoftwareVersion: "SW",
		Checksum:        "abc",
		Status:          pb.DeviceStatus_DEVICE_STATUS_UP,
	})

	rec := doRequest(t, rs, http.MethodGet, "/api/v1/devices/d1/diagnostics", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Diagnostics *pb.Diagnostics `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.GetHardwareVersion() != "HW" {
		t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
	}
}

func TestREST_GetDiagnostics_NotFound(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodGet, "/api/v1/devices/d1/diagnostics", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestREST_AddDevice(t *testing.T) {
	rs, st := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodPost, "/api/v1/devices", map[string]string{
		"id": "d9", "name": "Nine", "address": "10.0.0.9", "protocol": "rest",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if _, ok := st.GetDevice("d9"); !ok {
		t.Error("device not persisted")
	}
}

func TestREST_AddDevice_BadProtocol(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodPost, "/api/v1/devices", map[string]string{
		"id": "d9", "name": "Nine", "address": "10.0.0.9", "protocol": "bogus",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestREST_AddDevice_Duplicate(t *testing.T) {
	rs, st := newRESTTestServer(t)
	st.AddDevice(&model.Device{ID: "d1", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})
	rec := doRequest(t, rs, http.MethodPost, "/api/v1/devices", map[string]string{
		"id": "d1", "address": "10.0.0.1", "protocol": "rest",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestREST_AddDevice_InvalidBody(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestREST_RemoveDevice(t *testing.T) {
	rs, st := newRESTTestServer(t)
	st.AddDevice(&model.Device{ID: "d1", Address: "10.0.0.1", Protocol: pb.DeviceProtocol_DEVICE_PROTOCOL_REST})
	rec := doRequest(t, rs, http.MethodDelete, "/api/v1/devices/d1", nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if _, ok := st.GetDevice("d1"); ok {
		t.Error("device still present after delete")
	}
}

func TestREST_RemoveDevice_NotFound(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodDelete, "/api/v1/devices/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestREST_UnsupportedMethod(t *testing.T) {
	rs, _ := newRESTTestServer(t)
	rec := doRequest(t, rs, http.MethodPatch, "/api/v1/devices", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestParseProtocol(t *testing.T) {
	cases := []struct {
		in   string
		want pb.DeviceProtocol
		ok   bool
	}{
		{"grpc", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC, true},
		{"GRPC", pb.DeviceProtocol_DEVICE_PROTOCOL_GRPC, true},
		{"rest", pb.DeviceProtocol_DEVICE_PROTOCOL_REST, true},
		{"http", pb.DeviceProtocol_DEVICE_PROTOCOL_REST, true},
		{"REST", pb.DeviceProtocol_DEVICE_PROTOCOL_REST, true},
		{"bogus", pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED, false},
		{"", pb.DeviceProtocol_DEVICE_PROTOCOL_UNSPECIFIED, false},
	}
	for _, c := range cases {
		got, err := parseProtocol(c.in)
		if c.ok && err != nil {
			t.Errorf("parseProtocol(%q) error = %v, want nil", c.in, err)
		}
		if !c.ok && err == nil {
			t.Errorf("parseProtocol(%q) error = nil, want error", c.in)
		}
		if got != c.want {
			t.Errorf("parseProtocol(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "nope")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "nope" {
		t.Errorf("error body = %q, want nope", resp["error"])
	}
}

func TestStatusWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	sw.WriteHeader(http.StatusTeapot)
	if sw.status != http.StatusTeapot {
		t.Errorf("status = %d, want 418", sw.status)
	}
}
