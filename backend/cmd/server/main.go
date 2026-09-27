package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

// Resident representa un residente de la residencia de adultos mayores.
// Los datos básicos permiten identificar y gestionar la persona y su estado operativo.
type Resident struct {
	ID        interface{} `json:"id,omitempty" bson:"_id,omitempty"`
	FirstName string      `json:"firstName" bson:"firstName"`
	LastName  string      `json:"lastName" bson:"lastName"`
	Status    string      `json:"status" bson:"status"`
	CreatedAt *time.Time  `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt *time.Time  `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	ArchivedAt *time.Time  `json:"archivedAt,omitempty" bson:"archivedAt,omitempty"`
}

// ResidentCreate es el payload de creación de un residente.
// Se validan nombre y apellido para evitar registros vacíos o inconsistentes.
type ResidentCreate struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Status    string `json:"status"`
}

type ResidentPatch struct {
	FirstName *string `json:"firstName"`
	LastName  *string `json:"lastName"`
	Status    *string `json:"status"`
}

type ResidentContact struct {
	Name         string `json:"name,omitempty" bson:"name,omitempty"`
	Relationship string `json:"relationship,omitempty" bson:"relationship,omitempty"`
	Phone        string `json:"phone,omitempty" bson:"phone,omitempty"`
	Email        string `json:"email,omitempty" bson:"email,omitempty"`
}

type ResidentProfile struct {
	DateOfBirth    string          `json:"dateOfBirth,omitempty" bson:"dateOfBirth,omitempty"`
	Phone          string          `json:"phone,omitempty" bson:"phone,omitempty"`
	Email          string          `json:"email,omitempty" bson:"email,omitempty"`
	PrimaryContact ResidentContact `json:"primaryContact" bson:"primaryContact"`
	EmergencyContact ResidentContact `json:"emergencyContact" bson:"emergencyContact"`
}

type ContactPatch struct {
	Name         *string `json:"name"`
	Relationship *string `json:"relationship"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
}

type ProfilePatch struct {
	DateOfBirth      *string       `json:"dateOfBirth"`
	Phone            *string       `json:"phone"`
	Email            *string       `json:"email"`
	PrimaryContact   *ContactPatch `json:"primaryContact"`
	EmergencyContact *ContactPatch `json:"emergencyContact"`
}

// User representa un usuario del sistema con credenciales y permisos.
// El usuario administrador inicial se usa para pruebas de desarrollo y primer acceso del equipo.
type User struct {
	ID           interface{} `json:"id,omitempty" bson:"_id,omitempty"`
	Username     string      `json:"username" bson:"username"`
	PasswordHash string      `json:"-" bson:"passwordHash"`
	Role         string      `json:"role" bson:"role"`
	Permissions  []string    `json:"permissions,omitempty" bson:"permissions,omitempty"`
	CreatedAt    *time.Time  `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt    *time.Time  `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}

type AuthLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthRefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type authUserResponse struct {
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
}

type authSessionResponse struct {
	Token        string             `json:"token"`
	RefreshToken string            `json:"refreshToken"`
	User         authUserResponse   `json:"user"`
}

type jwtClaims struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
	jwt.RegisteredClaims
}

// rolePermissions define los permisos base para cada rol del sistema.
// Estos permisos sirven como base de control para la residencia y los módulos operativos.
var rolePermissions = map[string][]string{
	"admin": {
		"resident.read",
		"resident.create",
		"resident.update",
		"resident.delete",
		"user.read",
		"user.write",
		"room.read",
		"room.write",
		"medical.read",
		"medical.write",
		"medication.read",
		"medication.write",
		"billing.read",
		"billing.write",
		"document.read",
		"document.write",
		"audit.read",
		"dashboard.read",
	},
	"manager": {
		"resident.read",
		"resident.create",
		"resident.update",
		"resident.delete",
		"user.read",
		"room.read",
		"room.write",
		"medical.read",
		"billing.read",
		"audit.read",
		"dashboard.read",
	},
	"nurse": {
		"resident.read",
		"resident.update",
		"medical.read",
		"medical.write",
		"medication.read",
		"medication.write",
	},
	"kitchen": {
		"resident.read",
		"diet.read",
		"food.update",
	},
	"reception": {
		"resident.read",
		"resident.create",
		"resident.update",
		"room.read",
		"room.write",
	},
	"accounting": {
		"resident.read",
		"billing.read",
		"billing.write",
		"dashboard.read",
	},
}

// UserCreateRequest representa el cuerpo para crear un usuario del sistema.
type UserCreateRequest struct {
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
}

// UserUpdateRequest permite cambiar el rol o los permisos de un usuario existente.
type UserUpdateRequest struct {
	Role        *string  `json:"role,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
	Password    *string  `json:"password,omitempty"`
}

func main() {
	port := getenv("APP_PORT", "8080")
	mongoURI := getenv("MONGO_URI", "mongodb://mongodb:27017")
	dbName := getenv("MONGO_DATABASE", "cima")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("mongo ping: %v", err)
	}

	// collection almacena la información principal de residentes.
	collection := client.Database(dbName).Collection("residents")
	// userCollection guarda usuarios, roles y permisos para la autenticación del sistema.
	userCollection := client.Database(dbName).Collection("users")
	if err := seedDefaultAdminUser(context.Background(), userCollection); err != nil {
		log.Fatalf("seed admin user: %v", err)
	}

	r := chi.NewRouter()

	r.Use(corsMiddleware(getenv("CORS_ORIGIN", "http://localhost:5173")))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", func(w http.ResponseWriter, req *http.Request) {
			var input AuthLoginRequest
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			username := strings.TrimSpace(input.Username)
			password := input.Password
			if username == "" || password == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": username}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
				return
			}
			if err := verifyPassword(password, user.PasswordHash); err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
				return
			}

			jwtSecret := getenv("JWT_SECRET", "change-this-development-secret")
			accessToken, err := issueJWT(user.Username, user.Role, user.Permissions, jwtSecret, 15*time.Minute)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue access token"})
				return
			}
			refreshToken, err := issueJWT(user.Username, user.Role, user.Permissions, jwtSecret, 7*24*time.Hour)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue refresh token"})
				return
			}

			writeJSON(w, http.StatusOK, authSessionResponse{
				Token:        accessToken,
				RefreshToken: refreshToken,
				User: authUserResponse{
					Username:    user.Username,
					Role:        user.Role,
					Permissions: user.Permissions,
				},
			})
		})

		r.Post("/refresh", func(w http.ResponseWriter, req *http.Request) {
			var input AuthRefreshRequest
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			if strings.TrimSpace(input.RefreshToken) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refreshToken is required"})
				return
			}

			claims, err := parseJWT(input.RefreshToken, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": claims.Subject}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user no longer exists"})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
				return
			}

			newAccessToken, err := issueJWT(user.Username, user.Role, user.Permissions, getenv("JWT_SECRET", "change-this-development-secret"), 15*time.Minute)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue access token"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"token": newAccessToken})
		})

		r.Get("/me", func(w http.ResponseWriter, req *http.Request) {
			token, ok := bearerTokenFromRequest(req)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
				return
			}
			claims, err := parseJWT(token, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": claims.Subject}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
				return
			}

			writeJSON(w, http.StatusOK, authUserResponse{
				Username:    user.Username,
				Role:        user.Role,
				Permissions: user.Permissions,
			})
		})
	})

	r.Route("/api/v1/users", func(r chi.Router) {
		r.With(requirePermission("user.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			cursor, err := userCollection.Find(ctx, bson.M{})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
				return
			}
			defer cursor.Close(ctx)

			users := make([]User, 0)
			if err := cursor.All(ctx, &users); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decode users"})
				return
			}

			for i := range users {
				users[i].PasswordHash = ""
			}
			writeJSON(w, http.StatusOK, users)
		})

		r.With(requirePermission("user.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			var input UserCreateRequest
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			username := strings.TrimSpace(input.Username)
			password := input.Password
			role := strings.TrimSpace(input.Role)
			if username == "" || password == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
				return
			}
			if role == "" {
				role = "manager"
			}
			if !validUserRole(role) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
				return
			}

			var permissions []string
			if len(input.Permissions) > 0 {
				permissions = sanitizePermissions(input.Permissions)
			} else {
				permissions = permissionsForRole(role)
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var existing User
			if err := userCollection.FindOne(ctx, bson.M{"username": username}).Decode(&existing); err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
				return
			} else if err != mongo.ErrNoDocuments {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check user"})
				return
			}

			hash, err := hashPassword(password)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
				return
			}

			now := time.Now().UTC()
			result, err := userCollection.InsertOne(ctx, User{
				Username:     username,
				PasswordHash: hash,
				Role:         role,
				Permissions:  permissions,
				CreatedAt:    &now,
				UpdatedAt:    &now,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
				return
			}

			var created User
			if err := userCollection.FindOne(ctx, bson.M{"_id": result.InsertedID}).Decode(&created); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load created user"})
				return
			}
			created.PasswordHash = ""
			writeJSON(w, http.StatusCreated, created)
		})

		r.With(requirePermission("user.read")).Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
				return
			}
			user.PasswordHash = ""
			writeJSON(w, http.StatusOK, user)
		})

		r.With(requirePermission("user.write")).Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
				return
			}

			var patch UserUpdateRequest
			if !decodeStrictJSON(w, req, &patch) {
				return
			}

			updates := bson.M{}
			if patch.Role != nil {
				role := strings.TrimSpace(*patch.Role)
				if !validUserRole(role) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
					return
				}
				updates["role"] = role
				if patch.Permissions == nil {
					updates["permissions"] = permissionsForRole(role)
				}
			}
			if patch.Permissions != nil {
				updates["permissions"] = sanitizePermissions(*patch.Permissions)
			}
			if patch.Password != nil {
				password := strings.TrimSpace(*patch.Password)
				if password == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password cannot be empty"})
					return
				}
				hash, err := hashPassword(password)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
					return
				}
				updates["passwordHash"] = hash
			}
			if len(updates) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one user field is required"})
				return
			}
			updates["updatedAt"] = time.Now().UTC()

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			err = userCollection.FindOneAndUpdate(
				ctx,
				bson.M{"_id": objID},
				bson.M{"$set": updates},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			).Decode(&user)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
				return
			}
			user.PasswordHash = ""
			writeJSON(w, http.StatusOK, user)
		})
	})

	r.Route("/api/v1/dashboard", func(r chi.Router) {
		r.With(requirePermission("dashboard.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "dashboard ready", "module": "crm-dashboard"})
		})
	})

	r.Route("/api/v1/audit", func(r chi.Router) {
		r.With(requirePermission("audit.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "audit module ready"})
		})
	})

	r.Route("/api/v1/documents", func(r chi.Router) {
		r.With(requirePermission("document.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "documents module ready"})
		})
		r.With(requirePermission("document.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "document module not implemented yet"})
		})
	})

	r.Route("/api/v1/billing", func(r chi.Router) {
		r.With(requirePermission("billing.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "billing module ready"})
		})
		r.With(requirePermission("billing.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "billing module not implemented yet"})
		})
	})

	r.Route("/api/v1/rooms", func(r chi.Router) {
		r.With(requirePermission("room.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "rooms module ready"})
		})
		r.With(requirePermission("room.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "rooms module not implemented yet"})
		})
	})

	r.Route("/api/v1/medical", func(r chi.Router) {
		r.With(requirePermission("medical.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "medical module ready"})
		})
		r.With(requirePermission("medical.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "medical module not implemented yet"})
		})
	})

	r.Route("/api/v1/medication-events", func(r chi.Router) {
		r.With(requirePermission("medication.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "medication module ready"})
		})
		r.With(requirePermission("medication.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "medication module not implemented yet"})
		})
	})

	r.Route("/api/v1/residents", func(r chi.Router) {
		r.With(requirePermission("resident.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			cursor, err := collection.Find(ctx, bson.M{})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list residents"})
				return
			}
			defer cursor.Close(ctx)

			residents := make([]Resident, 0)
			if err := cursor.All(ctx, &residents); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decode residents"})
				return
			}
			writeJSON(w, http.StatusOK, residents)
		})

		r.With(requirePermission("resident.read")).Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
			residentID, ok := parseResidentID(w, req)
			if !ok {
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var resident Resident
			err := collection.FindOne(ctx, bson.M{"_id": residentID}).Decode(&resident)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "resident not found"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load resident"})
				return
			}
			writeJSON(w, http.StatusOK, resident)
		})

		r.With(requirePermission("resident.create")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			var input ResidentCreate
			if !decodeStrictJSON(w, req, &input) {
				return
			}

			resident := Resident{
				FirstName: strings.TrimSpace(input.FirstName),
				LastName:  strings.TrimSpace(input.LastName),
				Status:    strings.TrimSpace(input.Status),
			}
			if resident.FirstName == "" || resident.LastName == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "firstName and lastName are required"})
				return
			}
			if resident.Status == "" {
				resident.Status = "active"
			} else if !validResidentStatus(resident.Status) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or inactive"})
				return
			}
			now := time.Now().UTC()
			resident.CreatedAt = &now
			resident.UpdatedAt = &now

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			result, err := collection.InsertOne(ctx, resident)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create resident"})
				return
			}

			resident.ID = result.InsertedID
			writeJSON(w, http.StatusCreated, resident)
		})

		r.With(requirePermission("resident.update")).Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			residentID, ok := parseResidentID(w, req)
			if !ok {
				return
			}

			var patch ResidentPatch
			if !decodeStrictJSON(w, req, &patch) {
				return
			}

			updates := bson.M{}
			if patch.FirstName != nil {
				firstName := strings.TrimSpace(*patch.FirstName)
				if firstName == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "firstName cannot be empty"})
					return
				}
				updates["firstName"] = firstName
			}
			if patch.LastName != nil {
				lastName := strings.TrimSpace(*patch.LastName)
				if lastName == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lastName cannot be empty"})
					return
				}
				updates["lastName"] = lastName
			}
			if patch.Status != nil {
				status := strings.TrimSpace(*patch.Status)
				if !validResidentStatus(status) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or inactive"})
					return
				}
				updates["status"] = status
			}
			if len(updates) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one resident field is required"})
				return
			}
			updates["updatedAt"] = time.Now().UTC()

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var resident Resident
			err := collection.FindOneAndUpdate(
				ctx,
				bson.M{"_id": residentID},
				bson.M{"$set": updates},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			).Decode(&resident)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "resident not found"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update resident"})
				return
			}
			writeJSON(w, http.StatusOK, resident)
		})

		r.With(requirePermission("resident.delete")).Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
			residentID, ok := parseResidentID(w, req)
			if !ok {
				return
			}

			now := time.Now().UTC()
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			result, err := collection.UpdateOne(ctx, bson.M{"_id": residentID}, bson.M{"$set": bson.M{
				"status":     "archived",
				"archivedAt": now,
				"updatedAt":  now,
			}})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to archive resident"})
				return
			}
			if result.MatchedCount == 0 {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "resident not found"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		r.Route("/{id}/profile", func(r chi.Router) {
			r.With(requirePermission("resident.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
				residentID, ok := parseResidentID(w, req)
				if !ok {
					return
				}

				ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
				defer cancel()

				var resident struct {
					Profile ResidentProfile `bson:"profile"`
				}
				err := collection.FindOne(ctx, bson.M{"_id": residentID}).Decode(&resident)
				if err == mongo.ErrNoDocuments {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "resident not found"})
					return
				}
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load resident profile"})
					return
				}

				writeJSON(w, http.StatusOK, resident.Profile)
			})

			r.With(requirePermission("resident.update")).Patch("/", func(w http.ResponseWriter, req *http.Request) {
				residentID, ok := parseResidentID(w, req)
				if !ok {
					return
				}

				var patch ProfilePatch
				decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&patch); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile data"})
					return
				}
				if (patch.Email != nil && !validProfileEmail(*patch.Email)) ||
					(patch.PrimaryContact != nil && patch.PrimaryContact.Email != nil && !validProfileEmail(*patch.PrimaryContact.Email)) ||
					(patch.EmergencyContact != nil && patch.EmergencyContact.Email != nil && !validProfileEmail(*patch.EmergencyContact.Email)) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email must be a valid address"})
					return
				}

				updates := bson.M{}
				if patch.DateOfBirth != nil {
					if *patch.DateOfBirth != "" {
						if _, err := time.Parse("2006-01-02", *patch.DateOfBirth); err != nil {
							writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dateOfBirth must use YYYY-MM-DD"})
							return
						}
					}
					updates["profile.dateOfBirth"] = strings.TrimSpace(*patch.DateOfBirth)
				}
				if patch.Phone != nil {
					updates["profile.phone"] = strings.TrimSpace(*patch.Phone)
				}
				if patch.Email != nil {
					updates["profile.email"] = strings.TrimSpace(*patch.Email)
				}
				addContactUpdates(updates, "primaryContact", patch.PrimaryContact)
				addContactUpdates(updates, "emergencyContact", patch.EmergencyContact)
				if len(updates) == 0 {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one profile field is required"})
					return
				}

				ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
				defer cancel()

				result, err := collection.UpdateOne(ctx, bson.M{"_id": residentID}, bson.M{"$set": withUpdatedAt(updates)})
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update resident profile"})
					return
				}
				if result.MatchedCount == 0 {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "resident not found"})
					return
				}

				var resident struct {
					Profile ResidentProfile `bson:"profile"`
				}
				if err := collection.FindOne(ctx, bson.M{"_id": residentID}).Decode(&resident); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load updated resident profile"})
					return
				}
				writeJSON(w, http.StatusOK, resident.Profile)
			})
		})
	})

	log.Printf("CIMA API listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

// parseResidentID convierte el identificador recibido en la URL a un ObjectID de Mongo.
// Si el valor no es válido, responde con un error HTTP 400 y evita seguir con la operación.
func parseResidentID(w http.ResponseWriter, req *http.Request) (primitive.ObjectID, bool) {
	residentID, err := primitive.ObjectIDFromHex(chi.URLParam(req, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid resident id"})
		return primitive.NilObjectID, false
	}
	return residentID, true
}

// decodeStrictJSON asegura que el cuerpo venga como un único objeto JSON y que no haya campos desconocidos.
// Esto ayuda a evitar payloads maliciosos o inconsistentes en el backend.
func decodeStrictJSON(w http.ResponseWriter, req *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one JSON object"})
		return false
	}
	return true
}

// validResidentStatus valida los estados operativos permitidos para un residente.
func validResidentStatus(status string) bool {
	return status == "active" || status == "inactive"
}

// validUserRole comprueba si el rol recibido existe dentro del catálogo definido por el sistema.
func validUserRole(role string) bool {
	_, ok := rolePermissions[strings.TrimSpace(role)]
	return ok
}

// permissionsForRole devuelve los permisos asociados a un rol concreto.
func permissionsForRole(role string) []string {
	role = strings.TrimSpace(role)
	permissions, ok := rolePermissions[role]
	if !ok {
		return nil
	}
	result := make([]string, len(permissions))
	copy(result, permissions)
	return result
}

// sanitizePermissions filtra y ordena permisos para evitar duplicados o valores vacíos.
func sanitizePermissions(perms []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(perms))
	for _, permission := range perms {
		perm := strings.TrimSpace(permission)
		if perm == "" {
			continue
		}
		if _, exists := seen[perm]; exists {
			continue
		}
		seen[perm] = struct{}{}
		result = append(result, perm)
	}
	return result
}

// containsPermission ayuda a comprobar si un conjunto de permisos incluye uno específico.
func containsPermission(perms []string, permission string) bool {
	for _, item := range perms {
		if item == permission {
			return true
		}
	}
	return false
}

// seedDefaultAdminUser crea el usuario administrador inicial si aún no existe.
// Se utiliza como credencial base para entrar al sistema durante la etapa de desarrollo.
func seedDefaultAdminUser(ctx context.Context, collection *mongo.Collection) error {
	var user User
	if err := collection.FindOne(ctx, bson.M{"username": "admin"}).Decode(&user); err == nil {
		return nil
	} else if err != mongo.ErrNoDocuments {
		return err
	}

	hash, err := hashPassword("admin123")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = collection.InsertOne(ctx, User{
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
		Permissions: []string{
			"resident.read",
			"resident.create",
			"resident.update",
			"resident.delete",
			"user.read",
			"user.write",
		},
		CreatedAt: &now,
		UpdatedAt: &now,
	})
	return err
}

// hashPassword genera un hash bcrypt para almacenar contraseñas sin exponer texto plano.
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// verifyPassword compara una contraseña en texto plano con un hash bcrypt almacenado.
func verifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// issueJWT firma un token JWT con subject, role y permisos para el usuario autenticado.
func issueJWT(subject, role string, permissions []string, secret string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		Role:        role,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// parseJWT verifica el JWT y retorna sus claims si la firma y la expiración son válidas.
func parseJWT(tokenString, secret string) (*jwtClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid jwt claims")
	}
	return claims, nil
}

// bearerTokenFromRequest extrae el token JWT del encabezado Authorization con esquema Bearer.
func bearerTokenFromRequest(req *http.Request) (string, bool) {
	authorization := req.Header.Get("Authorization")
	if authorization == "" {
		return "", false
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return strings.TrimSpace(parts[1]), true
}

// requirePermission crea un middleware que valida el token del usuario y exige un permiso específico.
func requirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token, ok := bearerTokenFromRequest(req)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
				return
			}

			claims, err := parseJWT(token, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}

			if claims.Role == "admin" {
				next.ServeHTTP(w, req)
				return
			}

			hasPermission := containsPermission(claims.Permissions, permission)
			if !hasPermission {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden: missing permission"})
				return
			}

			next.ServeHTTP(w, req)
		})
	}
}

// addContactUpdates incorpora los cambios parciales de contacto dentro del subdocumento de perfil.
func addContactUpdates(updates bson.M, prefix string, contact *ContactPatch) {
	if contact == nil {
		return
	}
	if contact.Name != nil {
		updates["profile."+prefix+".name"] = strings.TrimSpace(*contact.Name)
	}
	if contact.Relationship != nil {
		updates["profile."+prefix+".relationship"] = strings.TrimSpace(*contact.Relationship)
	}
	if contact.Phone != nil {
		updates["profile."+prefix+".phone"] = strings.TrimSpace(*contact.Phone)
	}
	if contact.Email != nil {
		updates["profile."+prefix+".email"] = strings.TrimSpace(*contact.Email)
	}
}

func validProfileEmail(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func withUpdatedAt(updates bson.M) bson.M {
	result := bson.M{"updatedAt": time.Now().UTC()}
	for key, value := range updates {
		result[key] = value
	}
	return result
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func corsMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
