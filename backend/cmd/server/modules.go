package main

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const maxPrivateDocumentBytes = 8 << 20

// BillingEntry representa un cargo o pago del libro interno de cuenta corriente en pesos chilenos.
type BillingEntry struct {
	ID                primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	ResidentID        primitive.ObjectID `json:"residentId" bson:"residentId"`
	Kind              string             `json:"kind" bson:"kind"`
	Description       string             `json:"description" bson:"description"`
	AmountCLP         int64              `json:"amountClp" bson:"amountClp"`
	DueDate           *time.Time         `json:"dueDate,omitempty" bson:"dueDate,omitempty"`
	PaymentMethod     string             `json:"paymentMethod,omitempty" bson:"paymentMethod,omitempty"`
	Reference         string             `json:"reference,omitempty" bson:"reference,omitempty"`
	PaymentProvider   string             `json:"paymentProvider,omitempty" bson:"paymentProvider,omitempty"`
	ProviderReference string             `json:"providerReference,omitempty" bson:"providerReference,omitempty"`
	Simulated         bool               `json:"simulated,omitempty" bson:"simulated,omitempty"`
	CreatedBy         string             `json:"createdBy" bson:"createdBy"`
	CreatedAt         time.Time          `json:"createdAt" bson:"createdAt"`
}

// BillingEntryCreate define los datos comunes para registrar un cargo o abono.
type BillingEntryCreate struct {
	ResidentID    string `json:"residentId"`
	Description   string `json:"description"`
	AmountCLP     int64  `json:"amountClp"`
	DueDate       string `json:"dueDate"`
	PaymentMethod string `json:"paymentMethod"`
	Reference     string `json:"reference"`
}

// PrivateDocument conserva metadatos y contenido privado de un archivo en MongoDB.
type PrivateDocument struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	ResidentID  primitive.ObjectID `json:"residentId" bson:"residentId"`
	FileName    string             `json:"fileName" bson:"fileName"`
	Title       string             `json:"title" bson:"title"`
	Category    string             `json:"category" bson:"category"`
	ContentType string             `json:"contentType" bson:"contentType"`
	SizeBytes   int                `json:"sizeBytes" bson:"sizeBytes"`
	Content     []byte             `json:"-" bson:"content"`
	UploadedBy  string             `json:"uploadedBy" bson:"uploadedBy"`
	CreatedAt   time.Time          `json:"createdAt" bson:"createdAt"`
}

// PrivateDocumentCreate recibe metadatos y archivo codificado para persistirlo dentro de MongoDB.
type PrivateDocumentCreate struct {
	ResidentID    string `json:"residentId"`
	FileName      string `json:"fileName"`
	Title         string `json:"title"`
	Category      string `json:"category"`
	ContentType   string `json:"contentType"`
	ContentBase64 string `json:"contentBase64"`
}

// AuditEvent registra una mutación API exitosa sin almacenar el cuerpo de la solicitud.
type AuditEvent struct {
	ID         primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Actor      string             `json:"actor" bson:"actor"`
	Action     string             `json:"action" bson:"action"`
	Resource   string             `json:"resource" bson:"resource"`
	ResourceID string             `json:"resourceId,omitempty" bson:"resourceId,omitempty"`
	StatusCode int                `json:"statusCode" bson:"statusCode"`
	CreatedAt  time.Time          `json:"createdAt" bson:"createdAt"`
}

// BillingSummary resume cargos, pagos y saldo de una cuenta corriente.
type BillingSummary struct {
	ResidentID  primitive.ObjectID `json:"residentId" bson:"_id"`
	ChargesCLP  int64              `json:"chargesClp" bson:"chargesClp"`
	PaymentsCLP int64              `json:"paymentsClp" bson:"paymentsClp"`
	BalanceCLP  int64              `json:"balanceClp" bson:"balanceClp"`
}

// validateBillingEntryCreate normaliza y comprueba importes y referencias de cargos o pagos.
func validateBillingEntryCreate(input BillingEntryCreate, kind string) (BillingEntry, string) {
	entry := BillingEntry{
		Kind:          kind,
		Description:   strings.TrimSpace(input.Description),
		AmountCLP:     input.AmountCLP,
		PaymentMethod: strings.ToLower(strings.TrimSpace(input.PaymentMethod)),
		Reference:     strings.TrimSpace(input.Reference),
	}
	if kind != "charge" && kind != "payment" {
		return BillingEntry{}, "El tipo de movimiento no es válido."
	}
	residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
	if err != nil {
		return BillingEntry{}, "El identificador del residente no es válido."
	}
	entry.ResidentID = residentID
	if entry.Description == "" {
		return BillingEntry{}, "La descripción del movimiento es obligatoria."
	}
	if entry.AmountCLP <= 0 || entry.AmountCLP > 1_000_000_000_000 {
		return BillingEntry{}, "El monto debe ser un número positivo en pesos chilenos."
	}
	if kind == "charge" {
		if input.DueDate != "" {
			dueDate, err := time.Parse("2006-01-02", strings.TrimSpace(input.DueDate))
			if err != nil {
				return BillingEntry{}, "La fecha de vencimiento debe usar el formato AAAA-MM-DD."
			}
			entry.DueDate = &dueDate
		}
	} else {
		if entry.PaymentMethod == "" {
			return BillingEntry{}, "El medio de pago es obligatorio."
		}
		if len(entry.PaymentMethod) > 40 {
			return BillingEntry{}, "El medio de pago excede el largo permitido."
		}
	}
	return entry, ""
}

// validatePrivateDocumentCreate valida metadatos, tipo MIME y tamaño del archivo entrante.
func validatePrivateDocumentCreate(input PrivateDocumentCreate) (PrivateDocument, string) {
	residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(input.ResidentID))
	if err != nil {
		return PrivateDocument{}, "El identificador del residente no es válido."
	}
	fileName := filepath.Base(strings.ReplaceAll(strings.TrimSpace(input.FileName), "\\", "/"))
	if fileName == "." || fileName == "" || len(fileName) > 180 {
		return PrivateDocument{}, "El nombre del archivo no es válido."
	}
	title := strings.TrimSpace(input.Title)
	category := strings.ToLower(strings.TrimSpace(input.Category))
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if title == "" || len(title) > 140 {
		return PrivateDocument{}, "El título debe tener entre 1 y 140 caracteres."
	}
	if !validDocumentCategory(category) {
		return PrivateDocument{}, "La categoría debe ser identity, medical, contract, billing o other."
	}
	decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
	if err != nil || len(decoded) == 0 {
		return PrivateDocument{}, "El contenido del archivo no es válido."
	}
	if len(decoded) > maxPrivateDocumentBytes {
		return PrivateDocument{}, "El archivo supera el máximo de 8 MiB."
	}
	if !validDocumentContentType(contentType) {
		return PrivateDocument{}, "Solo se permiten archivos PDF, PNG, JPEG o texto plano."
	}
	detectedType := http.DetectContentType(decoded)
	if contentType == "image/jpg" {
		contentType = "image/jpeg"
	}
	if !strings.HasPrefix(detectedType, contentType) {
		return PrivateDocument{}, "El tipo declarado no coincide con el contenido del archivo."
	}
	return PrivateDocument{
		ResidentID:  residentID,
		FileName:    fileName,
		Title:       title,
		Category:    category,
		ContentType: contentType,
		SizeBytes:   len(decoded),
		Content:     decoded,
	}, ""
}

// validDocumentCategory comprueba las categorías documentales habilitadas para la residencia.
func validDocumentCategory(category string) bool {
	switch category {
	case "identity", "medical", "contract", "billing", "other":
		return true
	default:
		return false
	}
}

// validDocumentContentType limita la carga a formatos de lectura habituales y no ejecutables.
func validDocumentContentType(contentType string) bool {
	switch contentType {
	case "application/pdf", "image/png", "image/jpeg", "text/plain":
		return true
	default:
		return false
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status != 0 {
		return
	}
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(data)
}

// auditSuccessfulMutations registra cambios API exitosos sin persistir cuerpos ni datos clínicos.
func auditSuccessfulMutations(collection *mongo.Collection) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !strings.HasPrefix(req.URL.Path, "/api/v1/") || (req.Method != http.MethodPost && req.Method != http.MethodPatch && req.Method != http.MethodPut && req.Method != http.MethodDelete) {
				next.ServeHTTP(w, req)
				return
			}
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, req)
			if recorder.status < http.StatusOK || recorder.status >= http.StatusBadRequest {
				return
			}
			token, ok := bearerTokenFromRequest(req)
			if !ok {
				return
			}
			claims, err := parseJWT(token, jwtSecretValue())
			if err != nil {
				return
			}
			parts := strings.Split(strings.Trim(strings.TrimPrefix(req.URL.Path, "/api/v1/"), "/"), "/")
			if len(parts) == 0 || parts[0] == "" {
				return
			}
			resourceID := ""
			if len(parts) > 1 {
				if _, err := primitive.ObjectIDFromHex(parts[1]); err == nil {
					resourceID = parts[1]
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := collection.InsertOne(ctx, AuditEvent{
				Actor:      claims.Subject,
				Action:     strings.ToLower(req.Method),
				Resource:   parts[0],
				ResourceID: resourceID,
				StatusCode: recorder.status,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				log.Printf("audit event insert failed for %s %s: %v", req.Method, req.URL.Path, err)
			}
		})
	}
}

// registerModuleRoutes instala los endpoints protegidos de facturación, documentos y auditoría.
func registerModuleRoutes(router chi.Router, residents, billing, documents, audit, plans, taxDrafts, reconciliations *mongo.Collection, clamAVAddress, paymentMode string) {
	documentUploadLimiter := newRequestRateLimiter(12, time.Minute)
	router.Route("/api/v1/billing", func(r chi.Router) {
		r.With(requirePermission("billing.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
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
			cursor, err := billing.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(500))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar la cuenta corriente."})
				return
			}
			defer cursor.Close(ctx)
			entries := make([]BillingEntry, 0)
			if err := cursor.All(ctx, &entries); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los movimientos."})
				return
			}
			writeJSON(w, http.StatusOK, entries)
		})
		r.With(requirePermission("billing.read")).Get("/summary", func(w http.ResponseWriter, req *http.Request) {
			residentID, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.URL.Query().Get("residentId")))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del residente es obligatorio y debe ser válido."})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			pipeline := mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"residentId": residentID}}},
				{{Key: "$group", Value: bson.M{
					"_id":         residentID,
					"chargesClp":  bson.M{"$sum": bson.M{"$cond": []any{bson.M{"$eq": []any{"$kind", "charge"}}, "$amountClp", 0}}},
					"paymentsClp": bson.M{"$sum": bson.M{"$cond": []any{bson.M{"$eq": []any{"$kind", "payment"}}, "$amountClp", 0}}},
				}}},
				{{Key: "$project", Value: bson.M{"_id": 1, "chargesClp": 1, "paymentsClp": 1, "balanceClp": bson.M{"$subtract": []any{"$chargesClp", "$paymentsClp"}}}}},
			}
			var summary BillingSummary
			cursor, err := billing.Aggregate(ctx, pipeline)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo calcular el saldo."})
				return
			}
			defer cursor.Close(ctx)
			if cursor.Next(ctx) {
				if err := cursor.Decode(&summary); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo procesar el saldo."})
					return
				}
			} else if err := cursor.Err(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo procesar el saldo."})
				return
			} else {
				summary = BillingSummary{ResidentID: residentID}
			}
			var planTotal struct {
				TotalAmountCLP int64 `bson:"totalAmountClp"`
			}
			planCursor, err := plans.Aggregate(ctx, mongo.Pipeline{
				{{Key: "$match", Value: bson.M{"residentId": residentID}}},
				{{Key: "$group", Value: bson.M{"_id": nil, "totalAmountClp": bson.M{"$sum": "$totalAmountClp"}}}},
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo calcular el saldo de cuotas."})
				return
			}
			defer planCursor.Close(ctx)
			if planCursor.Next(ctx) {
				if err := planCursor.Decode(&planTotal); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo procesar el saldo de cuotas."})
					return
				}
			} else if err := planCursor.Err(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo procesar el saldo de cuotas."})
				return
			}
			summary.ChargesCLP += planTotal.TotalAmountCLP
			summary.BalanceCLP += planTotal.TotalAmountCLP
			writeJSON(w, http.StatusOK, summary)
		})
		for _, kind := range []string{"charge", "payment"} {
			entryKind := kind
			r.With(requirePermission("billing.write")).Post("/"+entryKind+"s", func(w http.ResponseWriter, req *http.Request) {
				var input BillingEntryCreate
				if !decodeStrictJSON(w, req, &input) {
					return
				}
				entry, message := validateBillingEntryCreate(input, entryKind)
				if message != "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
					return
				}
				ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
				defer cancel()
				var resident Resident
				if err := residents.FindOne(ctx, bson.M{"_id": entry.ResidentID}).Decode(&resident); err == mongo.ErrNoDocuments {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
					return
				} else if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar al residente."})
					return
				}
				entry.CreatedBy = requestUsername(req)
				entry.CreatedAt = time.Now().UTC()
				result, err := billing.InsertOne(ctx, entry)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo registrar el movimiento."})
					return
				}
				entry.ID = result.InsertedID.(primitive.ObjectID)
				writeJSON(w, http.StatusCreated, entry)
			})
		}
		registerBillingExperimentRoutes(r, residents, billing, plans, taxDrafts, reconciliations, paymentMode)
	})

	router.Route("/api/v1/documents", func(r chi.Router) {
		r.With(requirePermission("document.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
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
			cursor, err := documents.Find(ctx, filter, options.Find().SetProjection(bson.M{"content": 0}).SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(200))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron cargar los documentos."})
				return
			}
			defer cursor.Close(ctx)
			items := make([]PrivateDocument, 0)
			if err := cursor.All(ctx, &items); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los documentos."})
				return
			}
			writeJSON(w, http.StatusOK, items)
		})
		r.With(requirePermission("document.write"), documentUploadLimiter.Middleware).Post("/", func(w http.ResponseWriter, req *http.Request) {
			req.Body = http.MaxBytesReader(w, req.Body, 12<<20)
			var input PrivateDocumentCreate
			if !decodeStrictJSON(w, req, &input) {
				return
			}
			document, message := validatePrivateDocumentCreate(input)
			if message != "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 35*time.Second)
			defer cancel()
			var resident Resident
			if err := residents.FindOne(ctx, bson.M{"_id": document.ResidentID}).Decode(&resident); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró al residente."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo verificar al residente."})
				return
			}
			if err := scanDocumentWithClamAV(ctx, clamAVAddress, document.Content); err != nil {
				if errors.Is(err, errDocumentMalwareDetected) {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "El archivo fue rechazado por el análisis de seguridad."})
					return
				}
				log.Printf("document malware scan unavailable: %v", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "No se pudo analizar el archivo. Inténtalo nuevamente más tarde."})
				return
			}
			document.UploadedBy = requestUsername(req)
			document.CreatedAt = time.Now().UTC()
			result, err := documents.InsertOne(ctx, document)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo guardar el documento."})
				return
			}
			document.ID = result.InsertedID.(primitive.ObjectID)
			document.Content = nil
			writeJSON(w, http.StatusCreated, document)
		})
		r.With(requirePermission("document.read")).Get("/{id}/download", func(w http.ResponseWriter, req *http.Request) {
			id, err := primitive.ObjectIDFromHex(chi.URLParam(req, "id"))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "El identificador del documento no es válido."})
				return
			}
			ctx, cancel := context.WithTimeout(req.Context(), 8*time.Second)
			defer cancel()
			var document PrivateDocument
			if err := documents.FindOne(ctx, bson.M{"_id": id}).Decode(&document); err == mongo.ErrNoDocuments {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "No se encontró el documento."})
				return
			} else if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo cargar el documento."})
				return
			}
			w.Header().Set("Content-Type", document.ContentType)
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": document.FileName}))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "private, no-store")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(document.Content)
		})
	})

	router.Route("/api/v1/audit", func(r chi.Router) {
		r.With(requirePermission("audit.read")).Get("/", func(w http.ResponseWriter, req *http.Request) {
			filter := bson.M{}
			if actor := strings.TrimSpace(req.URL.Query().Get("actor")); actor != "" {
				filter["actor"] = actor
			}
			if resource := strings.TrimSpace(req.URL.Query().Get("resource")); resource != "" {
				filter["resource"] = resource
			}
			ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
			defer cancel()
			cursor, err := audit.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(300))
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudo consultar la auditoría."})
				return
			}
			defer cursor.Close(ctx)
			events := make([]AuditEvent, 0)
			if err := cursor.All(ctx, &events); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "No se pudieron procesar los eventos."})
				return
			}
			writeJSON(w, http.StatusOK, events)
		})
	})
}
