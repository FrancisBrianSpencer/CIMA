package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const maxReconciliationCSVBytes = 5 << 20
const maxReconciliationCSVRows = 2000

// InstallmentPlan guarda un calendario interno de cuotas que aporta al saldo, sin crear un DTE.
type InstallmentPlan struct {
	ID               primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	ResidentID       primitive.ObjectID `json:"residentId" bson:"residentId"`
	Description      string             `json:"description" bson:"description"`
	TotalAmountCLP   int64              `json:"totalAmountClp" bson:"totalAmountClp"`
	InstallmentCount int                `json:"installmentCount" bson:"installmentCount"`
	Installments     []Installment      `json:"installments" bson:"installments"`
	CreatedBy        string             `json:"createdBy" bson:"createdBy"`
	CreatedAt        time.Time          `json:"createdAt" bson:"createdAt"`
}

// Installment representa una cuota planificada, con vencimiento y monto entero en CLP.
type Installment struct {
	Number    int       `json:"number" bson:"number"`
	AmountCLP int64     `json:"amountClp" bson:"amountClp"`
	DueDate   time.Time `json:"dueDate" bson:"dueDate"`
}

// InstallmentPlanCreate describe las condiciones para generar un calendario de cuotas local.
type InstallmentPlanCreate struct {
	ResidentID       string `json:"residentId"`
	Description      string `json:"description"`
	TotalAmountCLP   int64  `json:"totalAmountClp"`
	InstallmentCount int    `json:"installmentCount"`
	FirstDueDate     string `json:"firstDueDate"`
}

// TaxDocumentDraft representa un borrador administrativo sin folio ni validez tributaria.
type TaxDocumentDraft struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	ResidentID     primitive.ObjectID `json:"residentId" bson:"residentId"`
	DocumentType   string             `json:"documentType" bson:"documentType"`
	Description    string             `json:"description" bson:"description"`
	AmountCLP      int64              `json:"amountClp" bson:"amountClp"`
	Status         string             `json:"status" bson:"status"`
	FiscalValidity string             `json:"fiscalValidity" bson:"fiscalValidity"`
	CreatedBy      string             `json:"createdBy" bson:"createdBy"`
	CreatedAt      time.Time          `json:"createdAt" bson:"createdAt"`
}

// TaxDocumentDraftCreate recibe datos administrativos para un borrador, sin simular emisión SII.
type TaxDocumentDraftCreate struct {
	ResidentID   string `json:"residentId"`
	DocumentType string `json:"documentType"`
	Description  string `json:"description"`
	AmountCLP    int64  `json:"amountClp"`
}

// MockPaymentRequest contiene solo los datos de prueba; nunca acepta datos de tarjeta o credenciales.
type MockPaymentRequest struct {
	ResidentID     string `json:"residentId"`
	Description    string `json:"description"`
	AmountCLP      int64  `json:"amountClp"`
	IdempotencyKey string `json:"idempotencyKey"`
}

// MockPaymentResult deja explícito que la autorización es simulada y no mueve dinero real.
type MockPaymentResult struct {
	Simulated bool         `json:"simulated"`
	Status    string       `json:"status"`
	Message   string       `json:"message"`
	Entry     BillingEntry `json:"entry"`
}

// ReconciliationBatch conserva resultados de comparación sin guardar la cartola original.
type ReconciliationBatch struct {
	ID         primitive.ObjectID  `json:"id" bson:"_id,omitempty"`
	ImportedBy string              `json:"importedBy" bson:"importedBy"`
	CreatedAt  time.Time           `json:"createdAt" bson:"createdAt"`
	Rows       []ReconciliationRow `json:"rows" bson:"rows"`
}

// ReconciliationRow retiene solo fecha, monto, referencia y resultado de conciliación.
type ReconciliationRow struct {
	Line                  int                 `json:"line" bson:"line"`
	Date                  time.Time           `json:"date" bson:"date"`
	AmountCLP             int64               `json:"amountClp" bson:"amountClp"`
	Reference             string              `json:"reference" bson:"reference"`
	Status                string              `json:"status" bson:"status"`
	MatchedBillingEntryID *primitive.ObjectID `json:"matchedBillingEntryId,omitempty" bson:"matchedBillingEntryId,omitempty"`
}

// validateInstallmentPlanCreate divide un monto CLP en cuotas iguales y ajusta la última por residuo.
func validateInstallmentPlanCreate(input InstallmentPlanCreate) (InstallmentPlan, string) {
	residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
	if err != nil {
		return InstallmentPlan{}, "El identificador del residente no es válido."
	}
	description := strings.TrimSpace(input.Description)
	if description == "" || len(description) > 140 {
		return InstallmentPlan{}, "La descripción debe tener entre 1 y 140 caracteres."
	}
	if input.TotalAmountCLP <= 0 || input.TotalAmountCLP > 1_000_000_000_000 {
		return InstallmentPlan{}, "El total debe ser un monto positivo en pesos chilenos."
	}
	if input.InstallmentCount < 2 || input.InstallmentCount > 36 || int64(input.InstallmentCount) > input.TotalAmountCLP {
		return InstallmentPlan{}, "La cantidad debe estar entre 2 y 36 cuotas y permitir montos enteros positivos."
	}
	firstDueDate, err := time.Parse("2006-01-02", strings.TrimSpace(input.FirstDueDate))
	if err != nil {
		return InstallmentPlan{}, "La primera fecha de vencimiento debe usar el formato AAAA-MM-DD."
	}
	baseAmount := input.TotalAmountCLP / int64(input.InstallmentCount)
	remainder := input.TotalAmountCLP % int64(input.InstallmentCount)
	installments := make([]Installment, input.InstallmentCount)
	for index := 0; index < input.InstallmentCount; index++ {
		installments[index] = Installment{
			Number:    index + 1,
			AmountCLP: baseAmount,
			DueDate:   addCalendarMonths(firstDueDate, index),
		}
	}
	installments[len(installments)-1].AmountCLP += remainder
	return InstallmentPlan{
		ResidentID:       residentID,
		Description:      description,
		TotalAmountCLP:   input.TotalAmountCLP,
		InstallmentCount: input.InstallmentCount,
		Installments:     installments,
	}, ""
}

// addCalendarMonths preserves the requested day when possible and clamps to the month's final day.
func addCalendarMonths(date time.Time, months int) time.Time {
	monthStart := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location()).AddDate(0, months, 0)
	lastDay := time.Date(monthStart.Year(), monthStart.Month()+1, 0, 0, 0, 0, 0, date.Location()).Day()
	day := date.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(monthStart.Year(), monthStart.Month(), day, 0, 0, 0, 0, date.Location())
}

// validateTaxDocumentDraftCreate only permits internal previews explicitly marked as non-fiscal.
func validateTaxDocumentDraftCreate(input TaxDocumentDraftCreate) (TaxDocumentDraft, string) {
	residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
	if err != nil {
		return TaxDocumentDraft{}, "El identificador del residente no es válido."
	}
	documentType := strings.ToLower(strings.TrimSpace(input.DocumentType))
	if documentType != "boleta" && documentType != "factura" {
		return TaxDocumentDraft{}, "El tipo de borrador debe ser boleta o factura."
	}
	description := strings.TrimSpace(input.Description)
	if description == "" || len(description) > 180 {
		return TaxDocumentDraft{}, "La descripción debe tener entre 1 y 180 caracteres."
	}
	if input.AmountCLP <= 0 || input.AmountCLP > 1_000_000_000_000 {
		return TaxDocumentDraft{}, "El monto debe ser positivo y estar expresado en pesos chilenos."
	}
	return TaxDocumentDraft{
		ResidentID:     residentID,
		DocumentType:   documentType,
		Description:    description,
		AmountCLP:      input.AmountCLP,
		Status:         "draft",
		FiscalValidity: "not_valid_for_tax_use",
	}, ""
}

// parseReconciliationCSV valida el formato local y compara filas con pagos registrados por monto y referencia.
func parseReconciliationCSV(ctx context.Context, reader io.Reader, billing *mongo.Collection) ([]ReconciliationRow, error) {
	rows, err := parseReconciliationCSVRows(reader)
	if err != nil {
		return nil, err
	}
	matchedIDs := make(map[primitive.ObjectID]struct{})
	for index := range rows {
		cursor, err := billing.Find(ctx, bson.M{
			"kind":      "payment",
			"amountClp": rows[index].AmountCLP,
			"reference": rows[index].Reference,
		}, options.Find().SetLimit(2))
		if err != nil {
			return nil, errors.New("No se pudo comparar la cartola con los pagos registrados.")
		}
		entries := make([]BillingEntry, 0, 2)
		if err := cursor.All(ctx, &entries); err != nil {
			_ = cursor.Close(ctx)
			return nil, errors.New("No se pudieron procesar las coincidencias de pago.")
		}
		_ = cursor.Close(ctx)
		if len(entries) == 0 {
			continue
		}
		if len(entries) > 1 {
			rows[index].Status = "ambiguous"
			continue
		}
		entry := entries[0]
		if _, alreadyMatched := matchedIDs[entry.ID]; !alreadyMatched {
			rows[index].Status = "matched"
			rows[index].MatchedBillingEntryID = &entry.ID
			matchedIDs[entry.ID] = struct{}{}
		} else {
			rows[index].Status = "duplicate_reference"
		}
	}
	return rows, nil
}

// parseReconciliationCSVRows valida y normaliza filas sintéticas sin almacenar la cartola original.
func parseReconciliationCSVRows(reader io.Reader) ([]ReconciliationRow, error) {
	csvReader := csv.NewReader(io.LimitReader(reader, maxReconciliationCSVBytes+1))
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil {
		return nil, errors.New("No se pudo leer el encabezado CSV.")
	}
	for index := range header {
		header[index] = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(header[index], "\ufeff")))
	}
	if len(header) != 4 || header[0] != "date" || header[1] != "amount_clp" || header[2] != "reference" || header[3] != "description" {
		return nil, errors.New("El CSV debe comenzar con: date,amount_clp,reference,description.")
	}
	rows := make([]ReconciliationRow, 0)
	for line := 2; ; line++ {
		values, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Error leyendo la línea %d del CSV.", line)
		}
		if len(rows) >= maxReconciliationCSVRows {
			return nil, errors.New("El CSV supera el máximo de 2000 filas.")
		}
		if len(values) != 4 {
			return nil, fmt.Errorf("La línea %d debe tener cuatro columnas.", line)
		}
		date, dateErr := time.Parse("2006-01-02", strings.TrimSpace(values[0]))
		amount, amountErr := parsePositiveCLP(strings.TrimSpace(values[1]))
		reference := strings.TrimSpace(values[2])
		if dateErr != nil || amountErr != nil || reference == "" || len(reference) > 120 {
			return nil, fmt.Errorf("La línea %d contiene fecha, monto o referencia inválidos.", line)
		}
		rows = append(rows, ReconciliationRow{Line: line, Date: date, AmountCLP: amount, Reference: reference, Status: "unmatched"})
	}
	return rows, nil
}

// parsePositiveCLP accepts unformatted integer pesos to keep CSV reconciliation deterministic.
func parsePositiveCLP(value string) (int64, error) {
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount <= 0 {
		return 0, errors.New("invalid CLP amount")
	}
	return amount, nil
}

// mockPaymentReference creates a stable provider reference from a caller idempotency key.
func mockPaymentReference(idempotencyKey string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(idempotencyKey)))
	return "local-mock-" + hex.EncodeToString(digest[:16])
}

// registerBillingExperimentRoutes instala cuotas, borradores no fiscales, pago simulado y conciliación CSV.
func registerBillingExperimentRoutes(r chi.Router, residents, billing, plans, taxDrafts, reconciliations *mongo.Collection, paymentMode string) {
	r.With(requirePermission("billing.read")).Get("/installment-plans", func(w http.ResponseWriter, req *http.Request) {
		filter := bson.M{}
		if residentID := strings.TrimSpace(req.URL.Query().Get("residentId")); residentID != "" {
			parsedID, err := primitive.ObjectIDFromHex(residentID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente no es válido."})
				return
			}
			filter["residentId"] = parsedID
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		cursor, err := plans.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(200))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron cargar las cuotas."})
			return
		}
		defer cursor.Close(ctx)
		items := make([]InstallmentPlan, 0)
		if err := cursor.All(ctx, &items); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar las cuotas."})
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
	r.With(requirePermission("billing.write")).Post("/installment-plans", func(w http.ResponseWriter, req *http.Request) {
		var input InstallmentPlanCreate
		if !decodeStrictJSON(w, req, &input) {
			return
		}
		plan, message := validateInstallmentPlanCreate(input)
		if message != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		var resident Resident
		if err := residents.FindOne(ctx, bson.M{"_id": plan.ResidentID}).Decode(&resident); err == mongo.ErrNoDocuments {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar al residente."})
			return
		}
		plan.CreatedBy = requestUsername(req)
		plan.CreatedAt = time.Now().UTC()
		result, err := plans.InsertOne(ctx, plan)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo guardar el plan de cuotas."})
			return
		}
		plan.ID = result.InsertedID.(primitive.ObjectID)
		writeJSON(w, http.StatusCreated, plan)
	})

	r.With(requirePermission("billing.read")).Get("/tax-document-drafts", func(w http.ResponseWriter, req *http.Request) {
		filter := bson.M{}
		if residentID := strings.TrimSpace(req.URL.Query().Get("residentId")); residentID != "" {
			parsedID, err := primitive.ObjectIDFromHex(residentID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente no es válido."})
				return
			}
			filter["residentId"] = parsedID
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		cursor, err := taxDrafts.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(200))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron cargar los borradores."})
			return
		}
		defer cursor.Close(ctx)
		items := make([]TaxDocumentDraft, 0)
		if err := cursor.All(ctx, &items); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los borradores."})
			return
		}
		writeJSON(w, http.StatusOK, items)
	})
	r.With(requirePermission("billing.write")).Post("/tax-document-drafts", func(w http.ResponseWriter, req *http.Request) {
		var input TaxDocumentDraftCreate
		if !decodeStrictJSON(w, req, &input) {
			return
		}
		draft, message := validateTaxDocumentDraftCreate(input)
		if message != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		var resident Resident
		if err := residents.FindOne(ctx, bson.M{"_id": draft.ResidentID}).Decode(&resident); err == mongo.ErrNoDocuments {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar al residente."})
			return
		}
		draft.CreatedBy = requestUsername(req)
		draft.CreatedAt = time.Now().UTC()
		result, err := taxDrafts.InsertOne(ctx, draft)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo guardar el borrador."})
			return
		}
		draft.ID = result.InsertedID.(primitive.ObjectID)
		writeJSON(w, http.StatusCreated, draft)
	})

	r.With(requirePermission("billing.write")).Post("/mock-payments", func(w http.ResponseWriter, req *http.Request) {
		if paymentMode != "mock" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "La pasarela está deshabilitada; solo se permite PAYMENT_MODE=mock."})
			return
		}
		var input MockPaymentRequest
		if !decodeStrictJSON(w, req, &input) {
			return
		}
		idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
		if len(idempotencyKey) < 8 || len(idempotencyKey) > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "La clave de idempotencia debe tener entre 8 y 100 caracteres."})
			return
		}
		providerReference := mockPaymentReference(idempotencyKey)
		entry, message := validateBillingEntryCreate(BillingEntryCreate{
			ResidentID:    input.ResidentID,
			Description:   input.Description,
			AmountCLP:     input.AmountCLP,
			PaymentMethod: "local-mock",
			Reference:     providerReference,
		}, "payment")
		if message != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		var existing BillingEntry
		if err := billing.FindOne(ctx, bson.M{"providerReference": providerReference}).Decode(&existing); err == nil {
			if existing.ResidentID != entry.ResidentID || existing.AmountCLP != entry.AmountCLP || existing.Description != entry.Description {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "La clave de idempotencia ya se usó con otros datos de pago."})
				return
			}
			writeJSON(w, http.StatusOK, MockPaymentResult{Simulated: true, Status: "simulated-approved", Message: "Pago local simulado; no se transfirieron fondos.", Entry: existing})
			return
		} else if err != mongo.ErrNoDocuments {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo comprobar el pago simulado."})
			return
		}
		var resident Resident
		if err := residents.FindOne(ctx, bson.M{"_id": entry.ResidentID}).Decode(&resident); err == mongo.ErrNoDocuments {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
			return
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar al residente."})
			return
		}
		entry.PaymentProvider = "local-mock"
		entry.ProviderReference = providerReference
		entry.Simulated = true
		entry.CreatedBy = requestUsername(req)
		entry.CreatedAt = time.Now().UTC()
		result, err := billing.InsertOne(ctx, entry)
		if err != nil {
			if findErr := billing.FindOne(ctx, bson.M{"providerReference": providerReference}).Decode(&existing); findErr == nil {
				writeJSON(w, http.StatusOK, MockPaymentResult{Simulated: true, Status: "simulated-approved", Message: "Pago local simulado; no se transfirieron fondos.", Entry: existing})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo registrar el pago simulado."})
			return
		}
		entry.ID = result.InsertedID.(primitive.ObjectID)
		writeJSON(w, http.StatusCreated, MockPaymentResult{Simulated: true, Status: "simulated-approved", Message: "Pago local simulado; no se transfirieron fondos.", Entry: entry})
	})

	r.With(requirePermission("billing.write")).Post("/reconciliation/import", func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(strings.ToLower(req.Header.Get("Content-Type")), "text/csv") {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "El archivo de conciliación debe enviarse como text/csv."})
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxReconciliationCSVBytes+1)
		data, err := io.ReadAll(req.Body)
		if err != nil || len(data) > maxReconciliationCSVBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "El CSV supera el máximo de 5 MiB."})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
		defer cancel()
		rows, err := parseReconciliationCSV(ctx, bytes.NewReader(data), billing)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		batch := ReconciliationBatch{ImportedBy: requestUsername(req), CreatedAt: time.Now().UTC(), Rows: rows}
		result, err := reconciliations.InsertOne(ctx, batch)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo guardar el resultado de conciliación."})
			return
		}
		batch.ID = result.InsertedID.(primitive.ObjectID)
		writeJSON(w, http.StatusCreated, batch)
	})
	r.With(requirePermission("billing.read")).Get("/reconciliation/batches", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
		defer cancel()
		cursor, err := reconciliations.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(50))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron cargar las conciliaciones."})
			return
		}
		defer cursor.Close(ctx)
		batches := make([]ReconciliationBatch, 0)
		if err := cursor.All(ctx, &batches); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar las conciliaciones."})
			return
		}
		writeJSON(w, http.StatusOK, batches)
	})
}
