package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Resident struct {
	ID        interface{} `json:"id,omitempty" bson:"_id,omitempty"`
	FirstName string      `json:"firstName" bson:"firstName"`
	LastName  string      `json:"lastName" bson:"lastName"`
	Status    string      `json:"status" bson:"status"`
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

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
			var resident Resident
			if err := json.NewDecoder(req.Body).Decode(&resident); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
				return
			}

			resident.FirstName = strings.TrimSpace(resident.FirstName)
			resident.LastName = strings.TrimSpace(resident.LastName)
			if resident.FirstName == "" || resident.LastName == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "firstName and lastName are required"})
				return
			}
			if resident.Status == "" {
				resident.Status = "active"
			}

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
	})

	log.Printf("CIMA API listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
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
