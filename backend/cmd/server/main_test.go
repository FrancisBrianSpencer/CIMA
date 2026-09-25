package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidResidentStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{status: "active", valid: true},
		{status: "inactive", valid: true},
		{status: "archived", valid: false},
		{status: "unknown", valid: false},
		{status: "", valid: false},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			if got := validResidentStatus(test.status); got != test.valid {
				t.Errorf("validResidentStatus(%q) = %t, want %t", test.status, got, test.valid)
			}
		})
	}
}

func TestDecodeStrictJSON(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantOK     bool
		wantStatus int
		wantName   string
	}{
		{
			name:       "valid object",
			body:       `{"name":"Ada"}`,
			wantOK:     true,
			wantStatus: http.StatusOK,
			wantName:   "Ada",
		},
		{
			name:       "unknown field",
			body:       `{"name":"Ada","admin":true}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "multiple JSON values",
			body:       `{"name":"Ada"} {}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed JSON",
			body:       `{"name":`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			var input struct {
				Name string `json:"name"`
			}

			gotOK := decodeStrictJSON(response, request, &input)
			if gotOK != test.wantOK {
				t.Fatalf("decodeStrictJSON() = %t, want %t", gotOK, test.wantOK)
			}
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantOK && input.Name != test.wantName {
				t.Errorf("decoded name = %q, want %q", input.Name, test.wantName)
			}
		})
	}
}
