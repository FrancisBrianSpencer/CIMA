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

// Room representa una habitación física y su disponibilidad operativa.
type Room struct {
	ID               primitive.ObjectID    `json:"id" bson:"_id,omitempty"`
	Code             string                `json:"code" bson:"code"`
	Capacity         int                   `json:"capacity" bson:"capacity"`
	Status           string                `json:"status" bson:"status"`
	OccupantIDs      []primitive.ObjectID  `json:"occupantIds" bson:"occupantIds"`
	Occupancy        int                   `json:"occupancy" bson:"-"`
	AssignmentHistory []RoomAssignmentEvent `json:"-" bson:"assignmentHistory,omitempty"`
	CreatedAt        *time.Time            `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt        *time.Time            `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}

// RoomAssignmentRequest identifica al residente que se asigna o libera.
type RoomAssignmentRequest struct {
	ResidentID string `json:"residentId"`
}

// RoomAssignmentEvent conserva la trazabilidad de asignación y liberación.
type RoomAssignmentEvent struct {
	ResidentID primitive.ObjectID `json:"residentId" bson:"residentId"`
	Action     string             `json:"action" bson:"action"`
	Actor      string             `json:"actor" bson:"actor"`
	ChangedAt  time.Time          `json:"changedAt" bson:"changedAt"`
}

// RoomCreate contiene los campos necesarios para registrar una habitación.
type RoomCreate struct {
	Code     string `json:"code"`
	Capacity int    `json:"capacity"`
	Status   string `json:"status"`
}

// RoomPatch permite actualizar parcialmente el código, capacidad o estado.
type RoomPatch struct {
	Code     *string `json:"code"`
	Capacity *int    `json:"capacity"`
	Status   *string `json:"status"`
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
	// roomCollection almacena el catálogo de habitaciones de la residencia.
	roomCollection := client.Database(dbName).Collection("rooms")
	if _, err := roomCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		log.Fatalf("create room code index: %v", err)
	}
	if _, err := roomCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "occupantIds", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	}); err != nil {
		log.Fatalf("create unique room occupant index: %v", err)
	}
	if _, err := roomCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "occupantIds", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true),
	}); err != nil {
		log.Fatalf("create unique room occupant index: %v", err)
	}
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
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El usuario y la contraseña son obligatorios."})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": username}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El usuario o la contraseña son incorrectos."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el usuario."})
				return
			}
			if err := verifyPassword(password, user.PasswordHash); err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El usuario o la contraseña son incorrectos."})
				return
			}

			jwtSecret := getenv("JWT_SECRET", "change-this-development-secret")
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
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El token de renovación es obligatorio."})
				return
			}

			claims, err := parseJWT(input.RefreshToken, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El token de renovación no es válido."})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": claims.Subject}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El usuario ya no existe."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el usuario."})
				return
			}

			newAccessToken, err := issueJWT(user.Username, user.Role, user.Permissions, getenv("JWT_SECRET", "change-this-development-secret"), 15*time.Minute)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo generar el token de acceso."})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"token": newAccessToken})
		})

		r.Get("/me", func(w http.ResponseWriter, req *http.Request) {
			token, ok := bearerTokenFromRequest(req)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Falta el token de acceso."})
				return
			}
			claims, err := parseJWT(token, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El token no es válido."})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"username": claims.Subject}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "No se encontró el usuario."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el usuario."})
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
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo listar a los usuarios."})
				return
			}
			defer cursor.Close(ctx)

			users := make([]User, 0)
			if err := cursor.All(ctx, &users); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los usuarios."})
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
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El usuario y la contraseña son obligatorios."})
				return
			}
			if role == "" {
				role = "manager"
			}
			if !validUserRole(role) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El rol no es válido."})
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
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Ese nombre de usuario ya está registrado."})
				return
			} else if err != mongo.ErrNoDocuments {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar el usuario."})
				return
			}

			hash, err := hashPassword(password)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo proteger la contraseña."})
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
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo crear el usuario."})
				return
			}

			var created User
			if err := userCollection.FindOne(ctx, bson.M{"_id": result.InsertedID}).Decode(&created); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el usuario creado."})
				return
			}
			created.PasswordHash = ""
			writeJSON(w, http.StatusCreated, created)
		})

		r.With(requirePermission("user.read")).Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del usuario no es válido."})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var user User
			if err := userCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró el usuario."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el usuario."})
				return
			}
			user.PasswordHash = ""
			writeJSON(w, http.StatusOK, user)
		})

		r.With(requirePermission("user.write")).Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "id")
			objID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del usuario no es válido."})
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
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El rol no es válido."})
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
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "La contraseña no puede estar vacía."})
					return
				}
				hash, err := hashPassword(password)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo proteger la contraseña."})
					return
				}
				updates["passwordHash"] = hash
			}
			if len(updates) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Debes indicar al menos un dato del usuario."})
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
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró el usuario."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo actualizar el usuario."})
				return
			}
			user.PasswordHash = ""
			writeJSON(w, http.StatusOK, user)
		})
	})

	r.Route("/api/v1/dashboard", func(r chi.Router) {
			r.With(requirePermission("dashboard.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "Panel disponible", "module": "crm-dashboard"})
		})
	})

	r.Route("/api/v1/audit", func(r chi.Router) {
		r.With(requirePermission("audit.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "Módulo de auditoría disponible"})
		})
	})

	r.Route("/api/v1/documents", func(r chi.Router) {
		r.With(requirePermission("document.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "Módulo de documentos disponible"})
		})
		r.With(requirePermission("document.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "El módulo de documentos aún no está implementado."})
		})
	})

	r.Route("/api/v1/billing", func(r chi.Router) {
		r.With(requirePermission("billing.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "Módulo de facturación disponible"})
		})
		r.With(requirePermission("billing.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "El módulo de facturación aún no está implementado."})
		})
	})

	r.Route("/api/v1/rooms", func(r chi.Router) {
		r.With(requirePermission("room.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			cursor, err := roomCollection.Find(ctx, bson.M{})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron listar las habitaciones."})
				return
			}
			defer cursor.Close(ctx)

			rooms := make([]Room, 0)
			if err := cursor.All(ctx, &rooms); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar las habitaciones."})
				return
			}
			for i := range rooms {
				setRoomOccupancy(&rooms[i])
			}
			writeJSON(w, http.StatusOK, rooms)
		})
		r.With(requirePermission("room.read")).Get("/{id}/history", func(w http.ResponseWriter, req *http.Request) {
			roomID, ok := parseRoomID(w, req)
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var room Room
			err := roomCollection.FindOne(ctx, bson.M{"_id": roomID}).Decode(&room)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró la habitación."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el historial de la habitación."})
				return
			}
			if room.AssignmentHistory == nil {
				room.AssignmentHistory = []RoomAssignmentEvent{}
			}
			writeJSON(w, http.StatusOK, room.AssignmentHistory)
		})
		r.With(requirePermission("room.write"), requirePermission("resident.read")).Post("/{id}/assign", func(w http.ResponseWriter, req *http.Request) {
			roomID, ok := parseRoomID(w, req)
			if !ok {
				return
			}
			var input RoomAssignmentRequest
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente no es válido."})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			var resident Resident
			err = collection.FindOne(ctx, bson.M{"_id": residentID}).Decode(&resident)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar al residente."})
				return
			}
			if resident.Status != "active" {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Solo se pueden asignar residentes activos."})
				return
			}

			var existingRoom Room
			err = roomCollection.FindOne(ctx, bson.M{"occupantIds": residentID}).Decode(&existingRoom)
			if err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "El residente ya está asignado a una habitación."})
				return
			}
			if err != mongo.ErrNoDocuments {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo comprobar la asignación actual."})
				return
			}

			now := time.Now().UTC()
			event := RoomAssignmentEvent{ResidentID: residentID, Action: "assigned", Actor: requestUsername(req), ChangedAt: now}
			filter := bson.M{
				"_id":    roomID,
				"status": "available",
				"$expr":  bson.M{"$lt": bson.A{roomOccupancyExpression(), "$capacity"}},
			}
			var room Room
			err = roomCollection.FindOneAndUpdate(
				ctx,
				filter,
				bson.M{
					"$addToSet": bson.M{"occupantIds": residentID},
					"$push":    bson.M{"assignmentHistory": event},
					"$set":     bson.M{"updatedAt": now},
				},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			).Decode(&room)
			if mongo.IsDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "El residente ya está asignado a otra habitación."})
				return
			}
			if err == mongo.ErrNoDocuments {
				var existing Room
				if findErr := roomCollection.FindOne(ctx, bson.M{"_id": roomID}).Decode(&existing); findErr == mongo.ErrNoDocuments {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró la habitación."})
					return
				}
				writeJSON(w, http.StatusConflict, map[string]string{"error": "La habitación no está disponible o alcanzó su capacidad."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo asignar al residente."})
				return
			}
			setRoomOccupancy(&room)
			writeJSON(w, http.StatusOK, room)
		})
		r.With(requirePermission("room.write"), requirePermission("resident.read")).Post("/{id}/release", func(w http.ResponseWriter, req *http.Request) {
			roomID, ok := parseRoomID(w, req)
			if !ok {
				return
			}
			var input RoomAssignmentRequest
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente no es válido."})
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			now := time.Now().UTC()
			event := RoomAssignmentEvent{ResidentID: residentID, Action: "released", Actor: requestUsername(req), ChangedAt: now}
			var room Room
			err = roomCollection.FindOneAndUpdate(
				ctx,
				bson.M{"_id": roomID, "occupantIds": residentID},
				bson.M{
					"$pull": bson.M{"occupantIds": residentID},
					"$push": bson.M{"assignmentHistory": event},
					"$set":  bson.M{"updatedAt": now},
				},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			).Decode(&room)
			if err == mongo.ErrNoDocuments {
				var existing Room
				if findErr := roomCollection.FindOne(ctx, bson.M{"_id": roomID}).Decode(&existing); findErr == mongo.ErrNoDocuments {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró la habitación."})
					return
				}
				writeJSON(w, http.StatusConflict, map[string]string{"error": "El residente no está asignado a esta habitación."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo liberar al residente."})
				return
			}
			setRoomOccupancy(&room)
			writeJSON(w, http.StatusOK, room)
		})
		r.With(requirePermission("room.read")).Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
			roomID, ok := parseRoomID(w, req)
			if !ok {
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			var room Room
			err := roomCollection.FindOne(ctx, bson.M{"_id": roomID}).Decode(&room)
			if err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró la habitación."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar la habitación."})
				return
			}
			setRoomOccupancy(&room)
			writeJSON(w, http.StatusOK, room)
		})
		r.With(requirePermission("room.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			var input RoomCreate
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			room, validationError := validateRoomCreate(input)
			if validationError != "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationError})
				return
			}
			now := time.Now().UTC()
			room.CreatedAt = &now
			room.UpdatedAt = &now
			room.OccupantIDs = []primitive.ObjectID{}

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			result, err := roomCollection.InsertOne(ctx, room)
			if mongo.IsDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Ya existe una habitación con ese código."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo crear la habitación."})
				return
			}
			room.ID = result.InsertedID.(primitive.ObjectID)
			setRoomOccupancy(&room)
			writeJSON(w, http.StatusCreated, room)
		})
		r.With(requirePermission("room.write")).Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			roomID, ok := parseRoomID(w, req)
			if !ok {
				return
			}
			var patch RoomPatch
			if !decodeStrictJSON(w, req, &patch) {
				return
			}

			updates := bson.M{}
			if patch.Code != nil {
				code := normalizeRoomCode(*patch.Code)
				if code == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El código de habitación es obligatorio."})
					return
				}
				updates["code"] = code
			}
			if patch.Capacity != nil {
				if *patch.Capacity < 1 {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "La capacidad debe ser al menos 1."})
					return
				}
				updates["capacity"] = *patch.Capacity
			}
			if patch.Status != nil {
				status := strings.TrimSpace(*patch.Status)
				if !validRoomStatus(status) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El estado debe ser available, maintenance o closed."})
					return
				}
				updates["status"] = status
			}
			if len(updates) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Debes indicar al menos un dato de la habitación."})
				return
			}
			updates["updatedAt"] = time.Now().UTC()

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			filter := bson.M{"_id": roomID}
			constraints := bson.A{}
			if patch.Capacity != nil {
				constraints = append(constraints, bson.M{"$lte": bson.A{roomOccupancyExpression(), *patch.Capacity}})
			}
			if patch.Status != nil && strings.TrimSpace(*patch.Status) != "available" {
				constraints = append(constraints, bson.M{"$eq": bson.A{roomOccupancyExpression(), 0}})
			}
			if len(constraints) == 1 {
				filter["$expr"] = constraints[0]
			} else if len(constraints) > 1 {
				filter["$expr"] = bson.M{"$and": constraints}
			}

			var room Room
			err := roomCollection.FindOneAndUpdate(
				ctx,
				filter,
				bson.M{"$set": updates},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			).Decode(&room)
			if mongo.IsDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Ya existe una habitación con ese código."})
				return
			}
			if err == mongo.ErrNoDocuments {
				var existing Room
				if findErr := roomCollection.FindOne(ctx, bson.M{"_id": roomID}).Decode(&existing); findErr == mongo.ErrNoDocuments {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró la habitación."})
					return
				}
				writeJSON(w, http.StatusConflict, map[string]string{"error": "No se puede reducir la capacidad por debajo de la ocupación ni cerrar una habitación ocupada."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo actualizar la habitación."})
				return
			}
			setRoomOccupancy(&room)
			writeJSON(w, http.StatusOK, room)
		})
	})

	r.Route("/api/v1/medical", func(r chi.Router) {
		r.With(requirePermission("medical.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "Módulo de atención clínica disponible"})
		})
		r.With(requirePermission("medical.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "El módulo de atención clínica aún no está implementado."})
		})
	})

	r.Route("/api/v1/medication-events", func(r chi.Router) {
		r.With(requirePermission("medication.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "Módulo de medicamentos disponible"})
		})
		r.With(requirePermission("medication.write")).Post("/", func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "El módulo de medicamentos aún no está implementado."})
		})
	})

	r.Route("/api/v1/residents", func(r chi.Router) {
		r.With(requirePermission("resident.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			cursor, err := collection.Find(ctx, bson.M{})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo listar a los residentes."})
				return
			}
			defer cursor.Close(ctx)

			residents := make([]Resident, 0)
			if err := cursor.All(ctx, &residents); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los residentes."})
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
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar al residente."})
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
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El nombre y el apellido son obligatorios."})
				return
			}
			if resident.Status == "" {
				resident.Status = "active"
			} else if !validResidentStatus(resident.Status) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El estado debe ser activo o inactivo."})
				return
			}
			now := time.Now().UTC()
			resident.CreatedAt = &now
			resident.UpdatedAt = &now

			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()

			result, err := collection.InsertOne(ctx, resident)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo crear al residente."})
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
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El nombre no puede estar vacío."})
					return
				}
				updates["firstName"] = firstName
			}
			if patch.LastName != nil {
				lastName := strings.TrimSpace(*patch.LastName)
				if lastName == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El apellido no puede estar vacío."})
					return
				}
				updates["lastName"] = lastName
			}
			if patch.Status != nil {
				status := strings.TrimSpace(*patch.Status)
				if !validResidentStatus(status) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El estado debe ser activo o inactivo."})
					return
				}
				updates["status"] = status
			}
			if len(updates) == 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Debes indicar al menos un dato del residente."})
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
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo actualizar al residente."})
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
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo archivar al residente."})
				return
			}
			if result.MatchedCount == 0 {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
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
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
					return
				}
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar la ficha del residente."})
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
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Los datos de la ficha no son válidos."})
					return
				}
				if (patch.Email != nil && !validProfileEmail(*patch.Email)) ||
					(patch.PrimaryContact != nil && patch.PrimaryContact.Email != nil && !validProfileEmail(*patch.PrimaryContact.Email)) ||
					(patch.EmergencyContact != nil && patch.EmergencyContact.Email != nil && !validProfileEmail(*patch.EmergencyContact.Email)) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Ingresa una dirección de correo electrónico válida."})
					return
				}

				updates := bson.M{}
				if patch.DateOfBirth != nil {
					if *patch.DateOfBirth != "" {
						if _, err := time.Parse("2006-01-02", *patch.DateOfBirth); err != nil {
							writeJSON(w, http.StatusBadRequest, map[string]string{"error": "La fecha de nacimiento debe usar el formato AAAA-MM-DD."})
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
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Debes indicar al menos un dato de la ficha."})
					return
				}

				ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
				defer cancel()

				result, err := collection.UpdateOne(ctx, bson.M{"_id": residentID}, bson.M{"$set": withUpdatedAt(updates)})
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo actualizar la ficha del residente."})
					return
				}
				if result.MatchedCount == 0 {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
					return
				}

				var resident struct {
					Profile ResidentProfile `bson:"profile"`
				}
				if err := collection.FindOne(ctx, bson.M{"_id": residentID}).Decode(&resident); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar la ficha actualizada."})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente no es válido."})
		return primitive.NilObjectID, false
	}
	return residentID, true
}

// parseRoomID valida el identificador de habitación recibido en la URL.
func parseRoomID(w http.ResponseWriter, req *http.Request) (primitive.ObjectID, bool) {
	roomID, err := primitive.ObjectIDFromHex(chi.URLParam(req, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador de la habitación no es válido."})
		return primitive.NilObjectID, false
	}
	return roomID, true
}

// requestUsername extrae la identidad del JWT ya autorizado por el middleware de la ruta.
func requestUsername(req *http.Request) string {
	token, ok := bearerTokenFromRequest(req)
	if !ok {
		return ""
	}
	claims, err := parseJWT(token, getenv("JWT_SECRET", "change-this-development-secret"))
	if err != nil {
		return ""
	}
	return claims.Subject
}

// roomOccupancyExpression calcula el número de residentes incluso en registros creados antes del campo.
func roomOccupancyExpression() bson.M {
	return bson.M{"$size": bson.M{"$ifNull": bson.A{"$occupantIds", bson.A{}}}}
}

// setRoomOccupancy prepara el conteo público sin exponer los identificadores de residentes.
func setRoomOccupancy(room *Room) {
	room.Occupancy = len(room.OccupantIDs)
}

// decodeStrictJSON asegura que el cuerpo venga como un único objeto JSON y que no haya campos desconocidos.
// Esto ayuda a evitar payloads maliciosos o inconsistentes en el backend.
func decodeStrictJSON(w http.ResponseWriter, req *http.Request, destination any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El contenido de la solicitud no es válido."})
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "La solicitud debe contener un único objeto JSON."})
		return false
	}
	return true
}

// validResidentStatus valida los estados operativos permitidos para un residente.
func validResidentStatus(status string) bool {
	return status == "active" || status == "inactive"
}

// normalizeRoomCode unifica mayúsculas y elimina espacios exteriores del código.
func normalizeRoomCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// validRoomStatus limita los estados a los definidos para el catálogo de habitaciones.
func validRoomStatus(status string) bool {
	return status == "available" || status == "maintenance" || status == "closed"
}

// validateRoomCreate normaliza y comprueba los datos de una habitación nueva.
func validateRoomCreate(input RoomCreate) (Room, string) {
	room := Room{
		Code:     normalizeRoomCode(input.Code),
		Capacity: input.Capacity,
		Status:   strings.TrimSpace(input.Status),
	}
	if room.Code == "" || len(room.Code) > 32 || strings.ContainsAny(room.Code, " \t\r\n") {
		return Room{}, "El código es obligatorio, debe tener hasta 32 caracteres y no contener espacios."
	}
	if room.Capacity < 1 {
		return Room{}, "La capacidad debe ser al menos 1."
	}
	if room.Status == "" {
		room.Status = "available"
	} else if !validRoomStatus(room.Status) {
		return Room{}, "El estado debe ser available, maintenance o closed."
	}
	return room, ""
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
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Falta el token de acceso."})
				return
			}

			claims, err := parseJWT(token, getenv("JWT_SECRET", "change-this-development-secret"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "El token no es válido."})
				return
			}

			if claims.Role == "admin" {
				next.ServeHTTP(w, req)
				return
			}

			hasPermission := containsPermission(claims.Permissions, permission)
			if !hasPermission {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "No tienes permiso para realizar esta acción."})
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
