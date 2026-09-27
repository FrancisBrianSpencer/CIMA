package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type integrationResident struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// TestAPIIntegration valida el flujo HTTP completo contra una API y base MongoDB aisladas.
func TestAPIIntegration(t *testing.T) {
	apiURL := strings.TrimRight(os.Getenv("API_BASE_URL"), "/")
	mongoURI := os.Getenv("MONGO_URI")
	databaseName := os.Getenv("MONGO_DATABASE")
	if apiURL == "" || mongoURI == "" || databaseName == "" {
		t.Skip("la integración requiere API_BASE_URL, MONGO_URI y MONGO_DATABASE")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	cancel()
	if err != nil {
		t.Fatalf("conectar a MongoDB de integración: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := client.Disconnect(cleanupContext); err != nil {
			t.Errorf("desconectar MongoDB de integración: %v", err)
		}
	})

	residentCollection := client.Database(databaseName).Collection("residents")
	roomCollection := client.Database(databaseName).Collection("rooms")
	userCollection := client.Database(databaseName).Collection("users")
	username := fmt.Sprintf("integracion-%s", primitive.NewObjectID().Hex())
	residentID := primitive.NilObjectID
	roomID := primitive.NilObjectID
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if residentID != primitive.NilObjectID {
			if _, err := residentCollection.DeleteOne(cleanupContext, bson.M{"_id": residentID}); err != nil {
				t.Errorf("eliminar residente de integración: %v", err)
			}
		}
		if roomID != primitive.NilObjectID {
			if _, err := roomCollection.DeleteOne(cleanupContext, bson.M{"_id": roomID}); err != nil {
				t.Errorf("eliminar habitación de integración: %v", err)
			}
		}
		if _, err := userCollection.DeleteOne(cleanupContext, bson.M{"username": username}); err != nil {
			t.Errorf("eliminar usuario de integración: %v", err)
		}
	})

	httpClient := &http.Client{Timeout: 5 * time.Second}
	adminToken := waitForIntegrationLogin(t, httpClient, apiURL)

	status, body := integrationRequest(t, httpClient, apiURL, http.MethodPost, "/api/v1/rooms/", adminToken, RoomCreate{
		Code:     "  prueba-a101  ",
		Capacity: 2,
	})
	if status != http.StatusCreated {
		t.Fatalf("crear habitación = %d, se esperaba 201: %s", status, body)
	}
	var room Room
	if err := json.Unmarshal(body, &room); err != nil {
		t.Fatalf("decodificar habitación creada: %v", err)
	}
	roomID = room.ID
	if room.Code != "PRUEBA-A101" || room.Status != "available" {
		t.Fatalf("habitación creada = {código: %q, estado: %q}, se esperaba PRUEBA-A101/available", room.Code, room.Status)
	}

	status, _ = integrationRequest(t, httpClient, apiURL, http.MethodPost, "/api/v1/rooms/", adminToken, RoomCreate{
		Code:     "prueba-a101",
		Capacity: 1,
	})
	if status != http.StatusConflict {
		t.Errorf("código de habitación duplicado = %d, se esperaba 409", status)
	}

	roomPath := "/api/v1/rooms/" + room.ID.Hex()
	status, _ = integrationRequest(t, httpClient, apiURL, http.MethodGet, roomPath, adminToken, nil)
	if status != http.StatusOK {
		t.Errorf("consultar habitación = %d, se esperaba 200", status)
	}
	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPatch, roomPath, adminToken, map[string]int{
		"capacity": 3,
	})
	if status != http.StatusOK {
		t.Errorf("actualizar habitación = %d, se esperaba 200: %s", status, body)
	}
	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPatch, roomPath, adminToken, map[string]int{
		"capacity": 0,
	})
	if status != http.StatusBadRequest {
		t.Errorf("capacidad inválida de habitación = %d, se esperaba 400: %s", status, body)
	}

	status, _ = integrationRequest(t, httpClient, apiURL, http.MethodGet, "/api/v1/residents/", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("consulta anónima de residentes = %d, se esperaba 401", status)
	}

	status, body := integrationRequest(t, httpClient, apiURL, http.MethodPost, "/api/v1/residents/", adminToken, map[string]string{
		"firstName": "Residente",
		"lastName":  "Integración",
	})
	if status != http.StatusCreated {
		t.Fatalf("crear residente = %d, se esperaba 201: %s", status, body)
	}
	var resident integrationResident
	if err := json.Unmarshal(body, &resident); err != nil {
		t.Fatalf("decodificar residente creado: %v", err)
	}
	residentID, err = primitive.ObjectIDFromHex(resident.ID)
	if err != nil {
		t.Fatalf("ID de residente no es ObjectID: %q", resident.ID)
	}

	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPatch, "/api/v1/residents/"+resident.ID, adminToken, map[string]string{
		"firstName": "Residente Actualizado",
	})
	if status != http.StatusOK {
		t.Fatalf("actualizar residente = %d, se esperaba 200: %s", status, body)
	}

	profilePath := "/api/v1/residents/" + resident.ID + "/profile/"
	status, _ = integrationRequest(t, httpClient, apiURL, http.MethodPatch, profilePath, adminToken, map[string]string{
		"dateOfBirth": "1935-05-27",
		"phone":       "+56912345678",
	})
	if status != http.StatusOK {
		t.Fatalf("actualizar ficha = %d, se esperaba 200", status)
	}

	status, body = integrationRequest(t, httpClient, apiURL, http.MethodGet, profilePath, adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("consultar ficha = %d, se esperaba 200", status)
	}
	var profile ResidentProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatalf("decodificar ficha: %v", err)
	}
	if profile.DateOfBirth != "1935-05-27" {
		t.Errorf("fecha de nacimiento = %q, se esperaba 1935-05-27", profile.DateOfBirth)
	}

	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPatch, profilePath, adminToken, map[string]string{
		"dateOfBirth": "1935-02-30",
	})
	if status != http.StatusBadRequest {
		t.Errorf("fecha inválida = %d, se esperaba 400: %s", status, body)
	}

	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPost, "/api/v1/users/", adminToken, map[string]string{
		"username": username,
		"password": "Temporal-Integracion-2026",
		"role":     "nurse",
	})
	if status != http.StatusCreated {
		t.Fatalf("crear usuario nurse = %d, se esperaba 201: %s", status, body)
	}

	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPost, "/auth/login", "", map[string]string{
		"username": username,
		"password": "Temporal-Integracion-2026",
	})
	if status != http.StatusOK {
		t.Fatalf("login nurse = %d, se esperaba 200: %s", status, body)
	}
	var nurseSession authSessionResponse
	if err := json.Unmarshal(body, &nurseSession); err != nil {
		t.Fatalf("decodificar sesión nurse: %v", err)
	}

	status, _ = integrationRequest(t, httpClient, apiURL, http.MethodGet, "/api/v1/residents/", nurseSession.Token, nil)
	if status != http.StatusOK {
		t.Errorf("nurse con resident.read = %d, se esperaba 200", status)
	}
	status, body = integrationRequest(t, httpClient, apiURL, http.MethodGet, "/api/v1/dashboard/", nurseSession.Token, nil)
	if status != http.StatusForbidden {
		t.Errorf("nurse sin dashboard.read = %d, se esperaba 403: %s", status, body)
	}
	status, body = integrationRequest(t, httpClient, apiURL, http.MethodGet, "/api/v1/rooms/", nurseSession.Token, nil)
	if status != http.StatusForbidden {
		t.Errorf("nurse sin room.read = %d, se esperaba 403: %s", status, body)
	}
	status, body = integrationRequest(t, httpClient, apiURL, http.MethodPost, "/api/v1/residents/", nurseSession.Token, map[string]string{
		"firstName": "No autorizado",
		"lastName":  "Prueba",
	})
	if status != http.StatusForbidden {
		t.Errorf("nurse sin resident.create = %d, se esperaba 403: %s", status, body)
	}
}

// waitForIntegrationLogin espera el arranque de la API aislada y obtiene una sesión admin.
func waitForIntegrationLogin(t *testing.T, client *http.Client, apiURL string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"username": "admin", "password": "admin123"})
	if err != nil {
		t.Fatalf("preparar login admin: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	var lastError error
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodPost, apiURL+"/auth/login", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("crear solicitud de login: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK {
				var session authSessionResponse
				if err := json.Unmarshal(body, &session); err != nil {
					t.Fatalf("decodificar sesión admin: %v", err)
				}
				return session.Token
			}
			lastError = fmt.Errorf("login respondió %d: %s", response.StatusCode, body)
		} else {
			lastError = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("la API de integración no quedó disponible: %v", lastError)
	return ""
}

// integrationRequest envía JSON opcional y retorna el código HTTP y el cuerpo de respuesta.
func integrationRequest(t *testing.T, client *http.Client, apiURL, method, path, token string, payload any) (int, []byte) {
	t.Helper()
	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("codificar solicitud de integración: %v", err)
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, apiURL+path, requestBody)
	if err != nil {
		t.Fatalf("crear solicitud %s %s: %v", method, path, err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("enviar solicitud %s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("leer respuesta %s %s: %v", method, path, err)
	}
	return response.StatusCode, body
}
