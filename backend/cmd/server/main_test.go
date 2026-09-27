package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestHashPasswordAndVerify(t *testing.T) {
	hash, err := hashPassword("StrongPass!123")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if hash == "" || hash == "StrongPass!123" {
		t.Fatalf("hashPassword() produced empty or plaintext hash: %q", hash)
	}
	if err := verifyPassword("StrongPass!123", hash); err != nil {
		t.Fatalf("verifyPassword() for valid password returned error: %v", err)
	}
	if err := verifyPassword("WrongPass!123", hash); err == nil {
		t.Fatal("verifyPassword() accepted an invalid password")
	}
}

func TestIssueAndParseJWT(t *testing.T) {
	token, err := issueJWT("admin", "admin", []string{"resident.read"}, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("issueJWT() error = %v", err)
	}
	claims, err := parseJWT(token, "test-secret")
	if err != nil {
		t.Fatalf("parseJWT() error = %v", err)
	}
	if claims.Subject != "admin" {
		t.Fatalf("claims.Subject = %q, want %q", claims.Subject, "admin")
	}
	if claims.Role != "admin" {
		t.Fatalf("claims.Role = %q, want %q", claims.Role, "admin")
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "resident.read" {
		t.Fatalf("claims.Permissions = %#v, want [resident.read]", claims.Permissions)
	}
}

func TestPermissionsForRole(t *testing.T) {
	perms := permissionsForRole("nurse")
	if len(perms) == 0 {
		t.Fatal("permissionsForRole() returned no permissions for nurse")
	}
	if !containsPermission(perms, "resident.read") {
		t.Fatalf("permissionsForRole(nurse) = %#v, want resident.read present", perms)
	}
	if len(permissionsForRole("invalid-role")) != 0 {
		t.Fatal("permissionsForRole() accepted an unknown role")
	}
}

func TestValidUserRole(t *testing.T) {
	if !validUserRole("manager") {
		t.Fatal("validUserRole(manager) = false, want true")
	}
	if validUserRole("unknown-role") {
		t.Fatal("validUserRole(unknown-role) = true, want false")
	}
}
