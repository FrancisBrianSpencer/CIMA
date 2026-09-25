package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/mail"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Resident struct {
	ID        interface{} `json:"id,omitempty" bson:"_id,omitempty"`
	FirstName string      `json:"firstName" bson:"firstName"`
	LastName  string      `json:"lastName" bson:"lastName"`
	Status    string      `json:"status" bson:"status"`
	CreatedAt *time.Time  `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt *time.Time  `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	ArchivedAt *time.Time  `json:"archivedAt,omitempty" bson:"archivedAt,omitempty"`
}

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
	DateOfBirth     *string       `json:"dateOfBirth"`
	Phone           *string       `json:"phone"`
	Email           *string       `json:"email"`
	PrimaryContact  *ContactPatch `json:"primaryContact"`
	EmergencyContact *ContactPatch `json:"emergencyContact"`
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

	collection := client.Database(dbName).Collection("residents")

	r := chi.NewRouter()

	r.Use(corsMiddleware(getenv("CORS_ORIGIN", "http://localhost:5173")))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1/residents", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
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

		r.Get("/{id}", func(w http.ResponseWriter, req *http.Request) {
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

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
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

		r.Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
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

		r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
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
			r.Get("/", func(w http.ResponseWriter, req *http.Request) {
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

			r.Patch("/", func(w http.ResponseWriter, req *http.Request) {
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

func parseResidentID(w http.ResponseWriter, req *http.Request) (primitive.ObjectID, bool) {
	residentID, err := primitive.ObjectIDFromHex(chi.URLParam(req, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid resident id"})
		return primitive.NilObjectID, false
	}
	return residentID, true
}

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

func validResidentStatus(status string) bool {
	return status == "active" || status == "inactive"
}

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
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
