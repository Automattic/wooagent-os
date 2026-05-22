package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type bodyTestStruct struct {
	Name string `json:"name"`
}

// decodeJSONBody must enforce maxRequestBodyBytes and surface a 413 with the
// "request_too_large" code so operators can distinguish a hostile/oversized
// body from a malformed-JSON 400. Defense against DSGWOO-1361's P2 #4.
func TestDecodeJSONBody_OverMaxBytes_Returns413(t *testing.T) {
	// Build a *valid* JSON value that's larger than the limit so the decoder
	// keeps reading until MaxBytesReader truncates — a malformed body would
	// short-circuit with a syntax error before the limit fires.
	oversized := `{"name":"` + strings.Repeat("a", maxRequestBodyBytes) + `"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(oversized))
	w := httptest.NewRecorder()

	var dst bodyTestStruct
	if ok := decodeJSONBody(w, req, &dst, "bad_body"); ok {
		t.Fatalf("expected decodeJSONBody to return false on oversized body")
	}
	if got := w.Code; got != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", got)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if env.Error.Code != "request_too_large" {
		t.Errorf("error.code = %q, want request_too_large", env.Error.Code)
	}
}

// Unknown fields must reject with the caller-supplied 400 code. Typo'd keys
// silently no-op'ing was the contract before this hardening pass and we want
// to surface them at the boundary.
func TestDecodeJSONBody_UnknownField_Returns400(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok","nope":1}`))
	w := httptest.NewRecorder()

	var dst bodyTestStruct
	if ok := decodeJSONBody(w, req, &dst, "bad_body"); ok {
		t.Fatalf("expected decodeJSONBody to return false on unknown field")
	}
	if got := w.Code; got != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if env.Error.Code != "bad_body" {
		t.Errorf("error.code = %q, want bad_body", env.Error.Code)
	}
	if !strings.Contains(env.Error.Message, "nope") {
		t.Errorf("error.message should name the unknown field; got %q", env.Error.Message)
	}
}

// Well-formed bodies pass through.
func TestDecodeJSONBody_Happy(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok"}`))
	w := httptest.NewRecorder()

	var dst bodyTestStruct
	if ok := decodeJSONBody(w, req, &dst, "bad_body"); !ok {
		t.Fatalf("expected decodeJSONBody to return true on valid body; status=%d", w.Code)
	}
	if dst.Name != "ok" {
		t.Errorf("dst.Name = %q, want ok", dst.Name)
	}
}
