package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const oauthStateLifetime = 10 * time.Minute

type oauthProviderConfig struct {
	clientID     string
	clientSecret string
	redirectURL  string
	authorizeURL string
	tokenURL     string
	userInfoURL  string
	scopes       []string
}

type oauthFlow struct {
	StateHash string    `bson:"stateHash"`
	Provider  string    `bson:"provider"`
	Verifier  string    `bson:"verifier"`
	ExpiresAt time.Time `bson:"expiresAt"`
}

type userOAuthIdentity struct {
	Provider       string             `json:"provider" bson:"provider"`
	ProviderUserID string             `json:"providerUserId" bson:"providerUserId"`
	UserID         primitive.ObjectID `json:"userId" bson:"userId"`
	Email          string             `json:"email,omitempty" bson:"email,omitempty"`
	Name           string             `json:"name,omitempty" bson:"name,omitempty"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt" bson:"updatedAt"`
}

type oauthLoginCode struct {
	CodeHash         string    `bson:"codeHash"`
	BrowserNonceHash string    `bson:"browserNonceHash"`
	Username         string    `bson:"username"`
	ExpiresAt        time.Time `bson:"expiresAt"`
}

type oauthProviderProfile struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// oauthConfig devuelve endpoints y credenciales del proveedor sin exponer secretos al navegador.
func oauthConfig(provider string) (oauthProviderConfig, bool) {
	switch provider {
	case "google":
		config := oauthProviderConfig{
			clientID:     strings.TrimSpace(os.Getenv("OAUTH_GOOGLE_CLIENT_ID")),
			clientSecret: strings.TrimSpace(os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET")),
			redirectURL:  strings.TrimSpace(os.Getenv("OAUTH_GOOGLE_REDIRECT_URI")),
			authorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
			tokenURL:     "https://oauth2.googleapis.com/token",
			userInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
			scopes:       []string{"openid", "email", "profile"},
		}
		return config, config.clientID != "" && config.clientSecret != "" && config.redirectURL != ""
	case "microsoft":
		tenant := strings.TrimSpace(os.Getenv("OAUTH_MICROSOFT_TENANT_ID"))
		if tenant == "" {
			tenant = "common"
		}
		baseURL := "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0"
		config := oauthProviderConfig{
			clientID:     strings.TrimSpace(os.Getenv("OAUTH_MICROSOFT_CLIENT_ID")),
			clientSecret: strings.TrimSpace(os.Getenv("OAUTH_MICROSOFT_CLIENT_SECRET")),
			redirectURL:  strings.TrimSpace(os.Getenv("OAUTH_MICROSOFT_REDIRECT_URI")),
			authorizeURL: baseURL + "/authorize",
			tokenURL:     baseURL + "/token",
			userInfoURL:  "https://graph.microsoft.com/oidc/userinfo",
			scopes:       []string{"openid", "profile", "email", "User.Read"},
		}
		return config, config.clientID != "" && config.clientSecret != "" && config.redirectURL != ""
	default:
		return oauthProviderConfig{}, false
	}
}

// oauthEnabledProviders informa únicamente qué proveedores están completamente configurados.
func oauthEnabledProviders() []string {
	providers := make([]string, 0, 2)
	for _, provider := range []string{"google", "microsoft"} {
		if _, enabled := oauthConfig(provider); enabled {
			providers = append(providers, provider)
		}
	}
	return providers
}

// randomOAuthValue genera un valor criptográficamente aleatorio para state, PKCE y códigos efímeros.
func randomOAuthValue(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// oauthDigest convierte valores efímeros en hashes para no persistirlos en claro.
func oauthDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// oauthPKCEChallenge produce el desafío S256 requerido al intercambiar el código OAuth.
func oauthPKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// registerOAuthRoutes instala inicio, callback, consulta de proveedores e intercambio de sesión OAuth.
func registerOAuthRoutes(router chi.Router, users, identities, flows, loginCodes *mongo.Collection) {
	router.Get("/auth/oauth/providers", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string][]string{"providers": oauthEnabledProviders()})
	})
	router.Get("/auth/oauth/{provider}/start", func(w http.ResponseWriter, req *http.Request) {
		provider := chi.URLParam(req, "provider")
		config, enabled := oauthConfig(provider)
		if !enabled {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "El proveedor de acceso no está configurado."})
			return
		}

		state, err := randomOAuthValue(32)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo iniciar la autenticación."})
			return
		}
		verifier, err := randomOAuthValue(32)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo iniciar la autenticación."})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		_, err = flows.InsertOne(ctx, oauthFlow{
			StateHash: oauthDigest(state),
			Provider:  provider,
			Verifier:  verifier,
			ExpiresAt: time.Now().UTC().Add(oauthStateLifetime),
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo iniciar la autenticación."})
			return
		}
		setOAuthCookie(w, oauthStateCookieName(provider), oauthDigest(state), "/auth/oauth/", oauthStateLifetime)

		query := url.Values{
			"client_id":             {config.clientID},
			"redirect_uri":          {config.redirectURL},
			"response_type":         {"code"},
			"scope":                 {strings.Join(config.scopes, " ")},
			"state":                 {state},
			"code_challenge":        {oauthPKCEChallenge(verifier)},
			"code_challenge_method": {"S256"},
		}
		if provider == "microsoft" {
			query.Set("response_mode", "query")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, req, config.authorizeURL+"?"+query.Encode(), http.StatusFound)
	})
	router.Get("/auth/oauth/{provider}/callback", func(w http.ResponseWriter, req *http.Request) {
		provider := chi.URLParam(req, "provider")
		config, enabled := oauthConfig(provider)
		if !enabled {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		state := req.URL.Query().Get("state")
		code := req.URL.Query().Get("code")
		cookie, cookieErr := req.Cookie(oauthStateCookieName(provider))
		clearOAuthCookie(w, oauthStateCookieName(provider), "/auth/oauth/")
		if state == "" || code == "" || req.URL.Query().Get("error") != "" || cookieErr != nil || subtle.ConstantTimeCompare([]byte(oauthDigest(state)), []byte(cookie.Value)) != 1 {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()
		var flow oauthFlow
		err := flows.FindOneAndDelete(ctx, bson.M{
			"stateHash": oauthDigest(state),
			"provider":  provider,
			"expiresAt": bson.M{"$gt": time.Now().UTC()},
		}).Decode(&flow)
		if err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}

		profile, err := fetchOAuthProfile(ctx, config, code, flow.Verifier)
		if err != nil || profile.Subject == "" || (provider == "google" && (!profile.EmailVerified || profile.Email == "")) {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		if !oauthEmailAllowed(profile.Email) {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}

		var identity userOAuthIdentity
		err = identities.FindOne(ctx, bson.M{"provider": provider, "providerUserId": profile.Subject}).Decode(&identity)
		if err == mongo.ErrNoDocuments {
			identity, err = createPendingOAuthUser(ctx, users, identities, provider, profile)
		}
		if err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}

		var user User
		if err := users.FindOne(ctx, bson.M{"_id": identity.UserID}).Decode(&user); err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		loginCode, err := randomOAuthValue(32)
		if err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		browserNonce, err := randomOAuthValue(32)
		if err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		_, err = loginCodes.InsertOne(ctx, oauthLoginCode{
			CodeHash:         oauthDigest(loginCode),
			BrowserNonceHash: oauthDigest(browserNonce),
			Username:         user.Username,
			ExpiresAt:        time.Now().UTC().Add(2 * time.Minute),
		})
		if err != nil {
			oauthCallbackRedirect(w, req, "#oauth=error")
			return
		}
		setOAuthCookie(w, "cima_oauth_exchange", browserNonce, "/auth/oauth/", 2*time.Minute)
		oauthCallbackRedirect(w, req, "#oauth=code&code="+url.QueryEscape(loginCode))
	})
	router.Post("/auth/oauth/exchange", func(w http.ResponseWriter, req *http.Request) {
		var input struct {
			Code string `json:"code"`
		}
		if !decodeStrictJSON(w, req, &input) {
			return
		}
		if input.Code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El código de acceso es obligatorio."})
			return
		}
		cookie, cookieErr := req.Cookie("cima_oauth_exchange")
		clearOAuthCookie(w, "cima_oauth_exchange", "/auth/oauth/")
		if cookieErr != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El código de acceso no pertenece a este navegador."})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		var login oauthLoginCode
		err := loginCodes.FindOneAndDelete(ctx, bson.M{
			"codeHash":         oauthDigest(input.Code),
			"browserNonceHash": oauthDigest(cookie.Value),
			"expiresAt":        bson.M{"$gt": time.Now().UTC()},
		}).Decode(&login)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El código de acceso expiró o ya fue utilizado."})
			return
		}
		var user User
		if err := users.FindOne(ctx, bson.M{"username": login.Username}).Decode(&user); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "No se encontró la cuenta asociada."})
			return
		}
		jwtSecret := jwtSecretValue()
		accessToken, err := issueJWT(user.Username, user.Role, user.Permissions, jwtSecret, 15*time.Minute)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo generar el token de acceso."})
			return
		}
		refreshToken, err := issueJWT(user.Username, user.Role, user.Permissions, jwtSecret, 7*24*time.Hour)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo generar el token de renovación."})
			return
		}
		writeJSON(w, http.StatusOK, authSessionResponse{
			Token:        accessToken,
			RefreshToken: refreshToken,
			User: authUserResponse{
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Email:       user.Email,
				Role:        user.Role,
				Permissions: user.Permissions,
			},
		})
	})
}

// fetchOAuthProfile intercambia el código PKCE y consulta el perfil usando el access token del proveedor.
func fetchOAuthProfile(ctx context.Context, config oauthProviderConfig, code, verifier string) (oauthProviderProfile, error) {
	form := url.Values{
		"client_id":     {config.clientID},
		"client_secret": {config.clientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {config.redirectURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthProviderProfile{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return oauthProviderProfile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return oauthProviderProfile{}, fmt.Errorf("OAuth token endpoint returned %d", response.StatusCode)
	}
	var tokens oauthTokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil || tokens.AccessToken == "" {
		return oauthProviderProfile{}, errors.New("OAuth provider returned an invalid token response")
	}

	profileRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, config.userInfoURL, nil)
	if err != nil {
		return oauthProviderProfile{}, err
	}
	profileRequest.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	profileResponse, err := client.Do(profileRequest)
	if err != nil {
		return oauthProviderProfile{}, err
	}
	defer profileResponse.Body.Close()
	if profileResponse.StatusCode != http.StatusOK {
		return oauthProviderProfile{}, fmt.Errorf("OAuth userinfo endpoint returned %d", profileResponse.StatusCode)
	}
	var profile oauthProviderProfile
	if err := json.NewDecoder(io.LimitReader(profileResponse.Body, 1<<20)).Decode(&profile); err != nil {
		return oauthProviderProfile{}, err
	}
	profile.Email = strings.ToLower(strings.TrimSpace(profile.Email))
	profile.Name = strings.TrimSpace(profile.Name)
	return profile, nil
}

// createPendingOAuthUser registra una identidad nueva sin asignarle permisos operativos.
func createPendingOAuthUser(ctx context.Context, users, identities *mongo.Collection, provider string, profile oauthProviderProfile) (userOAuthIdentity, error) {
	username := oauthUsername(provider, profile.Subject)
	now := time.Now().UTC()
	_, err := users.InsertOne(ctx, User{
		Username:    username,
		DisplayName: profile.Name,
		Email:       profile.Email,
		Role:        "pending",
		Permissions: []string{},
		CreatedAt:   &now,
		UpdatedAt:   &now,
	})
	if err != nil {
		var existing User
		if findErr := users.FindOne(ctx, bson.M{"username": username}).Decode(&existing); findErr != nil {
			return userOAuthIdentity{}, err
		}
	}
	identity := userOAuthIdentity{
		Provider:       provider,
		ProviderUserID: profile.Subject,
		Email:          profile.Email,
		Name:           profile.Name,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	var user User
	if err := users.FindOne(ctx, bson.M{"username": username}).Decode(&user); err != nil {
		return userOAuthIdentity{}, err
	}
	userID, ok := user.ID.(primitive.ObjectID)
	if !ok {
		return userOAuthIdentity{}, errors.New("OAuth user has an invalid local identifier")
	}
	identity.UserID = userID
	if _, err := identities.InsertOne(ctx, identity); err != nil {
		var existing userOAuthIdentity
		if findErr := identities.FindOne(ctx, bson.M{"provider": provider, "providerUserId": profile.Subject}).Decode(&existing); findErr == nil {
			return existing, nil
		}
		return userOAuthIdentity{}, err
	}
	return identity, nil
}

// oauthUsername genera un alias local estable sin usar el correo como identificador de autorización.
func oauthUsername(provider, subject string) string {
	digest := sha256.Sum256([]byte(provider + ":" + subject))
	return "oauth_" + provider + "_" + hex.EncodeToString(digest[:12])
}

// oauthEmailAllowed aplica la allowlist opcional de dominios a los nuevos y existentes accesos federados.
func oauthEmailAllowed(email string) bool {
	configured := strings.TrimSpace(os.Getenv("OAUTH_ALLOWED_DOMAINS"))
	if configured == "" {
		return true
	}
	parts := strings.Split(strings.ToLower(email), "@")
	if len(parts) != 2 || parts[0] == "" {
		return false
	}
	domain := strings.TrimSpace(parts[1])
	for _, allowed := range strings.Split(configured, ",") {
		if domain == strings.TrimPrefix(strings.TrimSpace(strings.ToLower(allowed)), "@") {
			return true
		}
	}
	return false
}

// oauthFrontendURL devuelve el destino de retorno configurado para completar el login en React.
func oauthFrontendURL() string {
	configured := strings.TrimSpace(os.Getenv("OAUTH_FRONTEND_REDIRECT_URI"))
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	return strings.TrimRight(getenv("CORS_ORIGIN", "http://localhost:5173"), "/") + "/"
}

// oauthCallbackRedirect evita almacenar la respuesta OAuth y el código efímero en logs o referrers.
func oauthCallbackRedirect(w http.ResponseWriter, req *http.Request, fragment string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, req, oauthFrontendURL()+fragment, http.StatusFound)
}

// oauthStateCookieName asigna una cookie independiente a cada proveedor para correlacionar el callback.
func oauthStateCookieName(provider string) string {
	return "cima_oauth_state_" + provider
}

// setOAuthCookie configura una cookie efímera no accesible desde JavaScript y limitada al flujo OAuth.
func setOAuthCookie(w http.ResponseWriter, name, value, path string, lifetime time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   int(lifetime.Seconds()),
		HttpOnly: true,
		Secure:   strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production"),
		SameSite: http.SameSiteLaxMode,
	})
}

// clearOAuthCookie elimina la cookie temporal después de usarla o rechazar el callback.
func clearOAuthCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production"),
		SameSite: http.SameSiteLaxMode,
	})
}
