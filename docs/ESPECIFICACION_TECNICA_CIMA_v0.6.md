---
title: "CIMA"
subtitle: "Control Interno y Monitoreo de Adultos Mayores"
author: "Especificación técnica"
date: "30 de septiembre de 2026"
lang: es-CL
---

# Especificación técnica

**Versión del documento:** 0.6  
**Fecha de corte:** 30 de septiembre de 2026  
**Estado:** backend y frontend con residentes, habitaciones, estancias, notas clínicas y eventos de medicación implementados; módulos de alimentación, facturación, documentos, auditoría y OAuth2 quedan como próximos incrementos.

## Historial

- **0.3:** ficha personal, contactos y validaciones de datos.
- **0.4:** CRUD de residentes, ficha editable, archivado lógico y timestamps.
- **0.5:** login JWT, renovación de sesión, usuarios/roles, RBAC por endpoint, dashboard inicial, habitaciones, historial de asignación y validación de módulos pendientes.
- **0.6:** estancias, notas clínicas y eventos de medicación implementados; actualización del estado real del proyecto y definición de arquitectura OAuth2 para Microsoft/Outlook y Google.

## 1. Propósito

CIMA es una plataforma interna para la operación de una residencia de adultos mayores. Su objetivo es centralizar la administración de residentes, habitación, estancia, atención clínica, medicamentos, pagos y comprobación de operaciones, manteniendo la seguridad en el backend y la trazabilidad de decisiones.

La capa de negocio es siempre la autoridad en validaciones y permisos. El frontend no se conecta a MongoDB ni ejecuta lógica crítica de negocio.

```text
Navegador React/TypeScript
        | HTTPS en producción; HTTP local en desarrollo
        v
API Go + Chi
        |
        +--> JWT/RBAC local
        +--> OAuth2 providers (Microsoft/Google)
        |
        v
MongoDB
```

## 2. Estado verificado del proyecto

| Área | Estado actual | Observación |
|---|---|---|
| Entorno | Docker Compose con MongoDB, API Go y frontend React/Vite funcionando. | El host solo requiere Docker Desktop y Git. |
| Autenticación | Login local, JWT de acceso y refresh, RBAC completo por permiso y perfil autenticado. | Requiere fortalecimiento para producción. |
| Residentes | CRUD completo, archivado lógico, ficha/identidad y contacto. | Falta búsqueda, paginación y restauración de archivos. |
| Habitaciones | Catálogo, capacidad, disponibilidad, ocupación, asignación/liberación, historial de cambios. | Falta mantenimiento o baja formal de habitaciones. |
| Estadías | Modelo, validación, creación y lectura implementados. | Falta integración completa con flujo financiero y auditoría. |
| Atención clínica | Notas clínicas con validación, colección y formulario UI. | Falta edición/eliminado con auditoría y permisos específicos por recurso. |
| Medicación | Eventos de administración con colección, validación y formulario UI. | Falta historial completo, marcación por residente y acciones de administración. |
| Dashboard | Vista base con módulos y métricas simples. | No es todavía un dashboard operativo completo. |
| Alimentación | Sin módulo funcional y sin permisos definidos reales. | Siguiente bloque pendiente. |
| Facturación | Rutas placeholder. | Requiere diseño de cargos, pagos y cuotas. |
| Documentos | Rutas placeholder. | Requiere almacenamiento seguro y permisos por documento. |
| Auditoría | GET protegido y trazabilidad parcial. | Necesita registro de acciones sensibles y minimización de datos. |
| OAuth2 | Pendiente de diseño e implementación. | Requerirá providers Microsoft y Google. |

## 3. Stack y ejecución

- **API:** Go 1.23, Chi, MongoDB Go Driver, JWT HS256, bcrypt y próximos proveedores OAuth2.
- **Frontend:** React 18 + TypeScript + Vite.
- **Datos:** MongoDB 7. Colecciones activas: `residents`, `users`, `rooms`, `stays`, `clinical_notes`, `medication_events`.
- **Ejecución:** Docker Compose en modo Docker-first.
- **Servicios locales:**
  - Frontend: `http://localhost:5173`
  - API: `http://localhost:8080`
  - Health: `GET /health`
  - MongoDB: `localhost:27017`

Comandos principales:

```powershell
docker compose up --build
docker compose ps
docker compose logs -f backend
docker compose logs -f frontend
```

No se requiere instalar Go, Node ni MongoDB en el host para el flujo normal.

## 4. Autenticación y sesiones actuales

### Autenticación local

Los usuarios se autentican con usuario/contraseña local y JWT.

```text
POST /auth/login     público
POST /auth/refresh   público con refresh token
GET  /auth/me        requiere token válido
```

Reglas vigentes:

- Access token: 15 minutos.
- Refresh token: 7 días.
- Contraseña: bcrypt.
- RBAC: permisos por ruta y middleware backend.
- Frontend: almacena tokens en sessionStorage y trata 401 como renovación o cierre de sesión.

### Riesgos actuales

- No hay revocación de refresh token.
- No hay logout centralizado del servidor.
- Los permisos quedan validados en el JWT hasta su expiración.
- Los usuarios con acceso local no tienen vinculación con proveedores externos.

## 5. Roles y permisos

Permisos actuales del sistema:

```text
resident.read       resident.create      resident.update      resident.delete
user.read           user.write
room.read           room.write
stay.read           stay.write
medical.read        medical.write
medication.read     medication.write
billing.read        billing.write
document.read       document.write
audit.read          dashboard.read
```

Roles relevantes:

| Rol | Acceso principal |
|---|---|
| `admin` | Control total del sistema. |
| `manager` | Residentes, habitaciones, seguimiento operativo, dashboard. |
| `nurse` | Residentes, notas clínicas y medicación. |
| `reception` | Residentes y habitaciones. |
| `accounting` | Cargos/pagos y dashboard. |

La UI oculta módulos según permisos, pero el backend sigue siendo la fuente de verdad.

## 6. API implementada

### Rutas actuales y estado

| Método y ruta | Permiso | Estado |
|---|---|---|
| `GET /health` | Público | Implementado. |
| `POST /auth/login` | Público | Implementado. |
| `POST /auth/refresh` | Público | Implementado. |
| `GET /auth/me` | Token válido | Implementado. |
| `GET /api/v1/users/` | `user.read` | Implementado. |
| `POST /api/v1/users/` | `user.write` | Implementado. |
| `GET /api/v1/residents/` | `resident.read` | Implementado. |
| `POST /api/v1/residents/` | `resident.create` | Implementado. |
| `PATCH /api/v1/residents/{id}` | `resident.update` | Implementado. |
| `GET /api/v1/rooms/` | `room.read` | Implementado. |
| `POST /api/v1/rooms/` | `room.write` | Implementado. |
| `POST /api/v1/rooms/{id}/assign` | `room.write` | Implementado. |
| `POST /api/v1/rooms/{id}/release` | `room.write` | Implementado. |
| `GET /api/v1/stays/` | `stay.read` | Implementado. |
| `POST /api/v1/stays/` | `stay.write` | Implementado. |
| `GET /api/v1/medical/` | `medical.read` | Implementado. |
| `POST /api/v1/medical/` | `medical.write` | Implementado. |
| `GET /api/v1/medication-events/` | `medication.read` | Implementado. |
| `POST /api/v1/medication-events/` | `medication.write` | Implementado. |
| `GET /api/v1/dashboard/` | `dashboard.read` | Base implementada. |
| `GET /api/v1/audit/` | `audit.read` | Base protegida. |
| `GET /api/v1/billing/` | `billing.read` | Placeholder. |
| `GET /api/v1/documents/` | `document.read` | Placeholder. |

## 7. Modelos clave implementados

### Resident

- `id`, `firstName`, `lastName`, `status`
- Campos de perfil y contacto
- Archiving logic con `archivedAt`

### Habitaciones

- `code`, `capacity`, `status`, `occupantIds`, `assignmentHistory`
- Historial de cambios por actor y timestamp
- Validaciones de capacidad y asignación activa

### Estadías

```go
type Stay struct {
    ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    ResidentID primitive.ObjectID `json:"residentId" bson:"residentId"`
    RoomID    primitive.ObjectID `json:"roomId" bson:"roomId"`
    Status    string             `json:"status" bson:"status"`
    CheckIn   *time.Time         `json:"checkIn,omitempty" bson:"checkIn,omitempty"`
    CheckOut  *time.Time         `json:"checkOut,omitempty" bson:"checkOut,omitempty"`
    Notes     string             `json:"notes,omitempty" bson:"notes,omitempty"`
}
```

### Nota clínica

```go
type ClinicalNote struct {
    ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    ResidentID primitive.ObjectID `json:"residentId" bson:"residentId"`
    Summary   string             `json:"summary" bson:"summary"`
    Severity  string             `json:"severity" bson:"severity"`
    Notes     string             `json:"notes,omitempty" bson:"notes,omitempty"`
    CreatedBy string             `json:"createdBy" bson:"createdBy"`
    CreatedAt *time.Time         `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
    UpdatedAt *time.Time         `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}
```

### Evento de medicación

```go
type MedicationEvent struct {
    ID         primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    ResidentID primitive.ObjectID `json:"residentId" bson:"residentId"`
    Medication string             `json:"medication" bson:"medication"`
    Dose       string             `json:"dose" bson:"dose"`
    Schedule   string             `json:"schedule,omitempty" bson:"schedule,omitempty"`
    Status     string             `json:"status" bson:"status"`
    Notes      string             `json:"notes,omitempty" bson:"notes,omitempty"`
    CreatedBy  string             `json:"createdBy" bson:"createdBy"`
    CreatedAt  *time.Time         `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
    UpdatedAt  *time.Time         `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}
```

## 8. Validación y pruebas actuales

### Backend

- Validación de payloads con `decodeStrictJSON`.
- Validación de fechas, estado de habitación, capacidad y permisos.
- Tests Go para auth, RBAC y validaciones de negocio.
- Integración con MongoDB aislado.

### Frontend

- Build de producción exitoso con Vite.
- Validación de permisos, interacción con API y carga/error/éxito.

## 9. Qué falta por completar

1. Alimentación como módulo operativo independiente.
2. Facturación, pagos, cargos y estados de cuenta.
3. Documentación privada y almacenamiento seguro.
4. Auditoría real y trazabilidad de cambios sensibles.
5. UX de administración de usuarios y roles.
6. OAuth2 para login con Microsoft/Outlook y Google.
7. Hardening antes de producción: HTTPS, CORS estricto, secretos por entorno, rate limiting, monitoring, backups.

## 10. Propuesta de integración OAuth2

### Objetivo

Permitir que un usuario pueda registrarse o iniciar sesión con una cuenta de Microsoft/Outlook o Google sin perder la arquitectura actual de JWT y RBAC del backend.

El diseño debe ser compatible con el modelo actual de usuarios locales y con la posibilidad de migrar a autenticación federada sin romper la base de datos existente.

### Principios de diseño

- El backend es el único punto de autenticación.
- Los proveedores OAuth2 solo validan identidad del usuario y devuelven claims.
- El sistema local conserva `username`, `role`, `permissions` y `passwordHash` para cuentas internas.
- El usuario externo se vincula a un registro local mediante email o subject provider.
- El login con proveedor externo es una opción, no reemplaza la autenticación local.
- La administración sigue realizándose con RBAC local.

### Modelos sugeridos

```go
type UserOAuthIdentity struct {
    ID              primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    UserID          primitive.ObjectID `json:"userId" bson:"userId"`
    Provider        string             `json:"provider" bson:"provider"` // google, microsoft
    ProviderUserID  string             `json:"providerUserId" bson:"providerUserId"`
    Email           string             `json:"email" bson:"email"`
    EmailVerified   bool               `json:"emailVerified" bson:"emailVerified"`
    Name            string             `json:"name,omitempty" bson:"name,omitempty"`
    PictureURL      string             `json:"pictureUrl,omitempty" bson:"pictureUrl,omitempty"`
    CreatedAt       *time.Time         `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
    UpdatedAt       *time.Time         `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}
```

Además, se recomienda una colección `user_oauth_identities` con índices por `provider`, `providerUserId`, `email` y `userId`.

### Flujo funcional

1. El usuario pulsa “Continuar con Google” o “Continuar con Microsoft”.
2. El frontend llama a `/auth/oauth/{provider}/start`.
3. El backend genera un `state` cifrado y redirige al endpoint de autorización del proveedor.
4. El proveedor devuelve un `code` al `redirect_uri`.
5. El backend intercambia `code` por `token` y obtiene user info con scopes mínimos.
6. El backend valida email y genera o enlaza un usuario local.
7. El backend emite JWT local para la sesión actual.
8. El frontend guarda el access token + refresh token y continúa normalmente.

### Endpoints propuestos

```text
GET  /auth/oauth/google/start
GET  /auth/oauth/google/callback
GET  /auth/oauth/microsoft/start
GET  /auth/oauth/microsoft/callback
POST /auth/oauth/link
POST /auth/oauth/unlink
GET  /auth/oauth/providers
```

### Mapeo de claims

| Proveedor | Claim principal | Uso |
|---|---|---|
| Google | `email`, `sub`, `name`, `picture` | Identidad y avatar. |
| Microsoft | `preferred_username`, `email`, `sub`, `name`, `oid` | Identidad y usuario único. |

### Regla de vinculación

- Si el email ya existe en `users` y no tiene vínculo OAuth, se puede enlazar con confirmación.
- Si el email ya tiene un `UserOAuthIdentity` del mismo provider, se inicia sesión automáticamente.
- Si el usuario ya existe con un mismo email pero sin OAuth, se puede pedir “enlazar cuenta” o “continuar con cuenta local”.
- Si el cliente no tiene cuenta local, se crea un usuario local con rol por defecto (por ejemplo `manager` o `nurse` según política), y se vincula con el proveedor.

### Seguridad

- `state` con firma/nonce para prevenir CSRF.
- `PKCE` requerido para OAuth2 con proveedores modernos.
- `redirect_uri` registrado y estrictamente validado.
- `client_secret` solo en backend.
- almacenamiento seguro de secretos en variables de entorno o secret manager.
- imposition de email verificado y mapeo de providerUserId.
- bloqueo de cuentas si el email no pertenece al dominio autorizado del centro.

### Variables de entorno propuestas

```env
OAUTH_GOOGLE_CLIENT_ID=...
OAUTH_GOOGLE_CLIENT_SECRET=...
OAUTH_GOOGLE_REDIRECT_URI=http://localhost:8080/auth/oauth/google/callback

OAUTH_MICROSOFT_CLIENT_ID=...
OAUTH_MICROSOFT_CLIENT_SECRET=...
OAUTH_MICROSOFT_TENANT_ID=common
OAUTH_MICROSOFT_REDIRECT_URI=http://localhost:8080/auth/oauth/microsoft/callback

OAUTH_SESSION_SECRET=...
OAUTH_ALLOWED_DOMAINS=example.com,midominio.cl
```

### Requerimientos de instalaciones

#### Google

- Crear proyecto en Google Cloud Console.
- Habilitar “Google Identity Services / OAuth 2.0 Client ID”.
- Añadir redirect URI autorizados.
- Solicitar scopes: `openid email profile`.

#### Microsoft / Outlook / Microsoft Live Account

- Crear app en Microsoft Entra ID / Azure AD.
- Registrar aplicación web o SPA según flujo.
- Configurar redirect URI.
- Solicitar scopes: `openid profile email User.Read`.
- Comprobar `tenant_id` y `common` para cuentas personales y empresariales.

### Compatibilidad con la estructura actual

La integración OAuth2 no reemplaza el modelo JWT actual, sino que se añade un “provider login” que emite el mismo JWT interno. Esto permite:

- no romper sessionStorage ni `auth.ts` existente;
- mantener permisos `role` y `permissions`; 
- dejar la funcionalidad de usuarios locales intacta;
- usar OAuth2 solo como capa de identidad externa.

## 11. Roadmap recomendado

### Fase 1 — OAuth2 base

- Registro de proveedores en backend.
- Endpoints de inicio/callback.
- Vínculo de usuario con email.
- Generación de JWT interno.
- UI de botón “Iniciar sesión con Google/Microsoft”.

### Fase 2 — administración y seguridad

- Enlace/desvinculación de cuentas por usuario.
- Solicitud de verificación de email.
- Restricción por dominio y roles.
- Auditoría de login externo.

### Fase 3 — producción

- Secret manager.
- Rate limiting y protección CSRF.
- Manejo de cuentas duplicadas y recuperación.
- Monitoreo de acceso y alertas.

## 12. Criterios de cierre del módulo OAuth2

Se considera cerrado cuando:

1. La autenticación con Google y Microsoft funciona en entorno Docker.
2. El backend valida `state`, `PKCE` y los emails autorizados.
3. Los usuarios se enlazan de manera segura a una cuenta local.
4. El frontend confirma registro/inicio de sesión con ambos proveedores.
5. La sesión resultante usa el mismo mecanismo JWT/RBAC ya implementado.
6. Existen pruebas unitarias e integración para login externo, enlace, rechazo y errores.
7. No hay secretos ni client IDs en Git ni en código fuente.

## 13. Conclusión

CIMA ya ha consolidado la base operativa y de seguridad local: residentes, habitaciones, estancias, atención clínica y medicación. La siguiente etapa estratégica debe ser la expansión funcional del módulo de alimentación y, paralelo a ello, la incorporación segura de OAuth2 para Microsoft/Outlook y Google, preservando la arquitectura actual, los permisos por rol y el flujo Docker-first del proyecto.
