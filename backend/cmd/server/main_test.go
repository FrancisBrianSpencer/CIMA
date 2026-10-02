package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
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

func TestValidateRoomCreate(t *testing.T) {
	tests := []struct {
		name       string
		input      RoomCreate
		wantCode   string
		wantStatus string
		wantError  bool
	}{
		{
			name:       "normalizes code and defaults availability",
			input:      RoomCreate{Code: "  a-101 ", Capacity: 2},
			wantCode:   "A-101",
			wantStatus: "available",
		},
		{
			name:      "rejects empty code",
			input:     RoomCreate{Capacity: 1},
			wantError: true,
		},
		{
			name:      "rejects code with spaces",
			input:     RoomCreate{Code: "A 101", Capacity: 1},
			wantError: true,
		},
		{
			name:      "rejects capacity below one",
			input:     RoomCreate{Code: "A-101", Capacity: 0},
			wantError: true,
		},
		{
			name:      "rejects unknown status",
			input:     RoomCreate{Code: "A-101", Capacity: 1, Status: "occupied"},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			room, message := validateRoomCreate(test.input)
			if (message != "") != test.wantError {
				t.Fatalf("validateRoomCreate() error = %q, want error %t", message, test.wantError)
			}
			if test.wantError {
				return
			}
			if room.Code != test.wantCode || room.Status != test.wantStatus {
				t.Errorf("room = {code: %q, status: %q}, want {code: %q, status: %q}", room.Code, room.Status, test.wantCode, test.wantStatus)
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
	if !validUserRole("pending") || len(permissionsForRole("pending")) != 0 {
		t.Fatal("pending role must be recognized without granting permissions")
	}
}

func TestRequirePermission(t *testing.T) {
	t.Setenv("JWT_SECRET", "rbac-test-secret")

	allowedToken, err := issueJWT("nurse", "nurse", []string{"resident.read"}, "rbac-test-secret", time.Hour)
	if err != nil {
		t.Fatalf("issueJWT() for permitted user returned error: %v", err)
	}
	forbiddenToken, err := issueJWT("nurse", "nurse", []string{"medical.read"}, "rbac-test-secret", time.Hour)
	if err != nil {
		t.Fatalf("issueJWT() for restricted user returned error: %v", err)
	}
	adminToken, err := issueJWT("admin", "admin", nil, "rbac-test-secret", time.Hour)
	if err != nil {
		t.Fatalf("issueJWT() for administrator returned error: %v", err)
	}

	tests := []struct {
		name           string
		token          string
		wantStatus     int
		wantNextCalled bool
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "invalid token", token: "not-a-jwt", wantStatus: http.StatusUnauthorized},
		{name: "missing permission", token: forbiddenToken, wantStatus: http.StatusForbidden},
		{name: "has permission", token: allowedToken, wantStatus: http.StatusOK, wantNextCalled: true},
		{name: "administrator bypass", token: adminToken, wantStatus: http.StatusOK, wantNextCalled: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})
			handler := requirePermission("resident.read")(next)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/residents/", nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if nextCalled != test.wantNextCalled {
				t.Errorf("next called = %t, want %t", nextCalled, test.wantNextCalled)
			}
		})
	}
}

func TestValidStayStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{status: "planned", valid: true},
		{status: "active", valid: true},
		{status: "completed", valid: true},
		{status: "cancelled", valid: false},
		{status: "", valid: false},
	}

	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			if got := validStayStatus(test.status); got != test.valid {
				t.Errorf("validStayStatus(%q) = %t, want %t", test.status, got, test.valid)
			}
		})
	}
}

func TestValidateStayCreate(t *testing.T) {
	residentID := primitive.NewObjectID().Hex()
	roomID := primitive.NewObjectID().Hex()
	checkIn := "2026-09-01"
	checkOut := "2026-09-15"

	stay, message := validateStayCreate(StayCreate{ResidentID: residentID, RoomID: roomID, CheckIn: checkIn, CheckOut: checkOut})
	if message != "" {
		t.Fatalf("validateStayCreate() unexpected error: %q", message)
	}
	if stay.Status != "planned" {
		t.Fatalf("stay.Status = %q, want planned", stay.Status)
	}
	if stay.CheckIn == nil || stay.CheckOut == nil {
		t.Fatal("validateStayCreate() returned nil dates")
	}
	if stay.CheckIn.Format("2006-01-02") != checkIn || stay.CheckOut.Format("2006-01-02") != checkOut {
		t.Fatalf("dates = %q/%q, want %q/%q", stay.CheckIn.Format("2006-01-02"), stay.CheckOut.Format("2006-01-02"), checkIn, checkOut)
	}

	_, message = validateStayCreate(StayCreate{ResidentID: residentID, RoomID: roomID, CheckIn: "2026-09-20", CheckOut: "2026-09-15"})
	if message == "" {
		t.Fatal("validateStayCreate() accepted a check-out before check-in")
	}
}

func TestValidateClinicalNoteCreate(t *testing.T) {
	residentID := primitive.NewObjectID().Hex()
	note, message := validateClinicalNoteCreate(ClinicalNoteCreate{ResidentID: residentID, Summary: "Revisión", Notes: "Sin novedades."})
	if message != "" {
		t.Fatalf("validateClinicalNoteCreate() unexpected error: %q", message)
	}
	if note.Summary != "Revisión" || note.Severity != "normal" {
		t.Fatalf("note = %+v, want summary=Revisión, severity=normal", note)
	}

	_, message = validateClinicalNoteCreate(ClinicalNoteCreate{ResidentID: residentID, Summary: "", Notes: "Sin novedades."})
	if message == "" {
		t.Fatal("validateClinicalNoteCreate() accepted empty summary")
	}
}

func TestValidateMedicationEventCreate(t *testing.T) {
	residentID := primitive.NewObjectID().Hex()
	event, message := validateMedicationEventCreate(MedicationEventCreate{ResidentID: residentID, Medication: "Paracetamol", Dose: "500 mg", Status: "given"})
	if message != "" {
		t.Fatalf("validateMedicationEventCreate() unexpected error: %q", message)
	}
	if event.Medication != "Paracetamol" || event.Status != "given" {
		t.Fatalf("event = %+v, want medication=Paracetamol and status=given", event)
	}

	_, message = validateMedicationEventCreate(MedicationEventCreate{ResidentID: residentID, Medication: "", Dose: "500 mg"})
	if message == "" {
		t.Fatal("validateMedicationEventCreate() accepted empty medication")
	}
}

func TestValidateDietPlanCreate(t *testing.T) {
	residentID := primitive.NewObjectID().Hex()
	plan, message := validateDietPlanCreate(DietPlanCreate{ResidentID: residentID, MealType: "lunch", Menu: "Sopa + pescado", Status: "planned"})
	if message != "" {
		t.Fatalf("validateDietPlanCreate() unexpected error: %q", message)
	}
	if plan.MealType != "lunch" || plan.Status != "planned" {
		t.Fatalf("plan = %+v, want mealType=lunch and status=planned", plan)
	}

	_, message = validateDietPlanCreate(DietPlanCreate{ResidentID: residentID, MealType: "invalid", Menu: "Sopa"})
	if message == "" {
		t.Fatal("validateDietPlanCreate() accepted invalid meal type")
	}
}

func TestOAuthPKCEChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := oauthPKCEChallenge(verifier); got != want {
		t.Fatalf("oauthPKCEChallenge() = %q, want %q", got, want)
	}
}

func TestOAuthConfigRequiresCompleteCredentials(t *testing.T) {
	t.Setenv("OAUTH_GOOGLE_CLIENT_ID", "google-client")
	t.Setenv("OAUTH_GOOGLE_CLIENT_SECRET", "google-secret")
	t.Setenv("OAUTH_GOOGLE_REDIRECT_URI", "http://localhost:8080/auth/oauth/google/callback")
	google, enabled := oauthConfig("google")
	if !enabled || google.authorizeURL == "" || google.tokenURL == "" {
		t.Fatalf("oauthConfig(google) = (%+v, %t), want configured provider", google, enabled)
	}

	t.Setenv("OAUTH_MICROSOFT_CLIENT_ID", "")
	if _, enabled := oauthConfig("microsoft"); enabled {
		t.Fatal("oauthConfig(microsoft) enabled without complete credentials")
	}
	if _, enabled := oauthConfig("other"); enabled {
		t.Fatal("oauthConfig() enabled an unsupported provider")
	}
}

func TestOAuthEmailAllowlist(t *testing.T) {
	t.Setenv("OAUTH_ALLOWED_DOMAINS", "example.com, care.example")
	if !oauthEmailAllowed("staff@EXAMPLE.com") {
		t.Fatal("oauthEmailAllowed() rejected an allowed domain")
	}
	if oauthEmailAllowed("staff@not-example.com") {
		t.Fatal("oauthEmailAllowed() accepted a non-allowed domain")
	}
	if oauthEmailAllowed("invalid-email") {
		t.Fatal("oauthEmailAllowed() accepted an invalid email")
	}
}

func TestOAuthUsernameIsStableAndProviderScoped(t *testing.T) {
	googleUsername := oauthUsername("google", "subject-123")
	if googleUsername != oauthUsername("google", "subject-123") {
		t.Fatal("oauthUsername() returned different usernames for the same identity")
	}
	if googleUsername == oauthUsername("microsoft", "subject-123") {
		t.Fatal("oauthUsername() did not distinguish providers")
	}
}

func TestOAuthCallbackRedirectDoesNotCacheOrLeakReferrer(t *testing.T) {
	t.Setenv("OAUTH_FRONTEND_REDIRECT_URI", "http://localhost:5173/")
	request := httptest.NewRequest(http.MethodGet, "/auth/oauth/google/callback", nil)
	response := httptest.NewRecorder()
	oauthCallbackRedirect(response, request, "#oauth=code&code=one-time")

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusFound)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := response.Header().Get("Location"); !strings.Contains(got, "#oauth=code&code=one-time") || strings.Contains(got, "?oauth=") {
		t.Fatalf("Location = %q, want OAuth data in fragment", got)
	}
}

func TestOAuthCookieSecurityAttributes(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	response := httptest.NewRecorder()
	setOAuthCookie(response, "cima_oauth_exchange", "nonce", "/auth/oauth/", time.Minute)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "cima_oauth_exchange" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/auth/oauth/" {
		t.Fatalf("OAuth cookie attributes = %+v, want HttpOnly, Secure, SameSite=Lax, and OAuth path", cookie)
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
