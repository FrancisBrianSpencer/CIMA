---
title: "CIMA"
subtitle: "Control Interno y Monitoreo de Adultos Mayores"
author: "Especificación técnica"
date: "30 de septiembre de 2026"
lang: es-CL
---

# Especificación técnica

**Versión del documento:** 0.7
**Fecha de corte:** 30 de septiembre de 2026  
**Estado:** backend y frontend con residentes, habitaciones, estancias, atención clínica, medicación y alimentación; OAuth2 base para Google y Microsoft implementado. Facturación, documentos y auditoría funcional siguen pendientes.

## Historial

- **0.3:** ficha personal, contactos y validaciones de datos.
- **0.4:** CRUD de residentes, ficha editable, archivado lógico y timestamps.
- **0.5:** login JWT, renovación de sesión, usuarios/roles, RBAC por endpoint, dashboard inicial, habitaciones, historial de asignación y validación de módulos pendientes.
- **0.6:** estancias, notas clínicas y eventos de medicación implementados; actualización del estado real del proyecto y definición de arquitectura OAuth2 para Microsoft/Outlook y Google.
- **0.7:** OAuth2 Google/Microsoft con `state`, PKCE S256, registro en rol pendiente, intercambio de código efímero por JWT local y configuración Docker por entorno.

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
| Autenticación | Login local y OAuth2 Google/Microsoft, JWT de acceso/refresh y RBAC por permiso. | Nuevas cuentas federadas quedan pendientes de aprobación; requiere hardening adicional para producción. |
| Residentes | CRUD completo, archivado lógico, ficha/identidad y contacto. | Falta búsqueda, paginación y restauración de archivos. |
| Habitaciones | Catálogo, capacidad, disponibilidad, ocupación, asignación/liberación, historial de cambios. | Falta mantenimiento o baja formal de habitaciones. |
| Estadías | Modelo, validación, creación y lectura implementados. | Falta integración completa con flujo financiero y auditoría. |
| Atención clínica | Notas clínicas con validación, colección y formulario UI. | Falta edición/eliminado con auditoría y permisos específicos por recurso. |
| Medicación | Eventos de administración con colección, validación y formulario UI. | Falta historial completo, marcación por residente y acciones de administración. |
| Dashboard | Vista base con módulos y métricas simples. | No es todavía un dashboard operativo completo. |
| Alimentación | Planes por residente y turno con API, permisos y UI. | Ampliar con restricciones, alergias y dietas clínicas estructuradas. |
| Facturación | Rutas placeholder. | Requiere diseño de cargos, pagos y cuotas. |
| Documentos | Rutas placeholder. | Requiere almacenamiento seguro y permisos por documento. |
| Auditoría | GET protegido y trazabilidad parcial. | Necesita registro de acciones sensibles y minimización de datos. |
| OAuth2 | Inicio/callback, perfiles Google/Microsoft, colección de identidades y canje de sesión implementados. | Credenciales externas requieren configuración; enlace federado a cuentas locales existentes queda pendiente. |

## 3. Stack y ejecución

- **API:** Go 1.23, Chi, MongoDB Go Driver, JWT HS256, bcrypt y OAuth2 Google/Microsoft.
- **Frontend:** React 18 + TypeScript + Vite.
- **Datos:** MongoDB 7. Colecciones activas: `residents`, `users`, `rooms`, `stays`, `clinical_notes`, `medication_events`, `diet_plans`, `user_oauth_identities`, `oauth_flows` y `oauth_login_codes`.
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
GET  /auth/oauth/providers
GET  /auth/oauth/{google|microsoft}/start
GET  /auth/oauth/{google|microsoft}/callback
POST /auth/oauth/exchange
```

Reglas vigentes:

- Access token: 15 minutos.
- Refresh token: 7 días.
- Contraseña: bcrypt.
- RBAC: permisos por ruta y middleware backend.
- Frontend: almacena tokens en sessionStorage y trata 401 como renovación o cierre de sesión.
- OAuth2: `state` aleatorio con hash almacenado, cookie `HttpOnly`/`SameSite=Lax` para correlación, PKCE S256, secretos solo en backend y código de sesión de un solo uso vinculado al navegador.
- Registro federado: se crea un usuario `pending` y se inicia sesión automáticamente, pero no obtiene permisos ni acceso a datos hasta que un administrador le asigne un rol.
- La administración de usuarios permite revisar nombre/correo del proveedor y asignar un rol aprobado.
- No se vincula automáticamente una identidad por correo a una cuenta local preexistente.

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
| `pending` | Sin permisos; requiere aprobación administrativa. |

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
| `GET /api/v1/diet-plans/` | `diet.read` | Implementado. |
| `POST /api/v1/diet-plans/` | `food.update` | Implementado. |
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

1. Facturación, pagos, cargos y estados de cuenta.
2. Documentación privada y almacenamiento seguro.
3. Auditoría real y trazabilidad de cambios sensibles.
4. Enlace/desvinculación de proveedores a cuentas locales existentes.
5. Verificación real de login OAuth con credenciales registradas en Google y Entra ID.
6. Hardening antes de producción: HTTPS, rate limiting, monitoring, backups y gestión centralizada de secretos.

## 10. Integración OAuth2 implementada

### Objetivo

Permitir que un usuario pueda registrarse o iniciar sesión con una cuenta de Microsoft/Outlook o Google sin perder la arquitectura actual de JWT y RBAC del backend.

El diseño debe ser compatible con el modelo actual de usuarios locales y con la posibilidad de migrar a autenticación federada sin romper la base de datos existente.

### Principios de diseño

- El backend es el único punto de autenticación.
- Los proveedores OAuth2 solo validan identidad del usuario y devuelven claims.
- El sistema local conserva `username`, `role`, `permissions` y `passwordHash` para cuentas internas.
- La identidad externa se vincula a un usuario local por `provider + subject`; nunca se enlaza automáticamente solo por email.
- El login con proveedor externo es una opción, no reemplaza la autenticación local.
- El registro crea un usuario `pending` sin permisos; un administrador debe asignar un rol aprobado desde la pantalla de usuarios.
- Las credenciales de OAuth existen únicamente en el backend mediante variables de entorno.

### Identidad persistida

```go
type UserOAuthIdentity struct {
    ID              primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    UserID          primitive.ObjectID `json:"userId" bson:"userId"`
    Provider        string             `json:"provider" bson:"provider"` // google, microsoft
    ProviderUserID  string             `json:"providerUserId" bson:"providerUserId"`
    Email           string             `json:"email" bson:"email"`
    Name            string             `json:"name,omitempty" bson:"name,omitempty"`
    CreatedAt       time.Time          `json:"createdAt" bson:"createdAt"`
    UpdatedAt       time.Time          `json:"updatedAt" bson:"updatedAt"`
}
```

La colección `user_oauth_identities` tiene un índice único compuesto por `provider` y `providerUserId`, más un índice por `userId`. Los flujos OAuth y los códigos de canje se guardan con hash y vencimiento TTL.

### Flujo funcional

1. El usuario pulsa “Continuar con Google” o “Continuar con Microsoft”.
2. El frontend llama a `/auth/oauth/{provider}/start`.
3. El backend crea `state` aleatorio, challenge PKCE S256 y cookie `HttpOnly`/`SameSite=Lax`, y redirige al proveedor.
4. El proveedor devuelve un `code` al `redirect_uri`.
5. El backend intercambia `code` por `token` y obtiene user info con scopes mínimos.
6. El backend resuelve la identidad por `provider + subject`; para una identidad nueva crea un usuario local `pending` sin permisos.
7. El backend emite JWT local también para el rol `pending`, que no tiene permisos; la UI muestra el estado de aprobación sin habilitar datos protegidos.
8. El callback devuelve un código efímero vinculado al navegador; el frontend lo canjea y guarda los JWT en `sessionStorage`.

### Endpoints propuestos

```text
GET  /auth/oauth/google/start
GET  /auth/oauth/google/callback
GET  /auth/oauth/microsoft/start
GET  /auth/oauth/microsoft/callback
POST /auth/oauth/link
POST /auth/oauth/unlink
GET  /auth/oauth/providers
POST /auth/oauth/exchange
```

### Mapeo de claims

| Proveedor | Claim principal | Uso |
|---|---|---|
| Google | `email`, `sub`, `name`, `picture` | Identidad y avatar. |
| Microsoft | `preferred_username`, `email`, `sub`, `name`, `oid` | Identidad y usuario único. |

### Regla de identidad y aprobación

- Una identidad ya registrada por el mismo proveedor inicia sesión si su usuario local tiene un rol aprobado.
- El email no se usa como llave para asociar una identidad a cuentas locales existentes.
- Las nuevas cuentas requieren aprobación explícita y asignación de rol desde administración de usuarios.

### Seguridad

- `state` aleatorio, persistido como hash y vinculado a cookie `HttpOnly` para prevenir CSRF.
- `PKCE` requerido para OAuth2 con proveedores modernos.
- `redirect_uri` fijo por configuración de backend, nunca recibido desde el navegador.
- `client_secret` solo en backend.
- almacenamiento seguro de secretos en variables de entorno o secret manager.
- código de sesión de vida corta, uso único y ligado a una cookie temporal del mismo navegador.
- allowlist opcional de dominios mediante `OAUTH_ALLOWED_DOMAINS`; no sustituye la aprobación de cuenta.

### Variables de entorno propuestas

```env
OAUTH_GOOGLE_CLIENT_ID=...
OAUTH_GOOGLE_CLIENT_SECRET=...
OAUTH_GOOGLE_REDIRECT_URI=http://localhost:8080/auth/oauth/google/callback

OAUTH_MICROSOFT_CLIENT_ID=...
OAUTH_MICROSOFT_CLIENT_SECRET=...
OAUTH_MICROSOFT_TENANT_ID=common
OAUTH_MICROSOFT_REDIRECT_URI=http://localhost:8080/auth/oauth/microsoft/callback
OAUTH_FRONTEND_REDIRECT_URI=http://localhost:5173/
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

### Fase 1 — OAuth2 base (implementada; falta validar credenciales reales)

- Registro de proveedores en backend.
- Endpoints de inicio/callback.
- Registro por subject del proveedor con aprobación administrativa.
- Generación de JWT interno.
- UI de botón “Iniciar sesión con Google/Microsoft”.

### Fase 2 — administración y seguridad

- Enlace/desvinculación de cuentas por usuario.
- Solicitud de verificación de email.
- Restricción opcional por dominio y asignación de roles aprobados.
- Auditoría de login externo.

### Fase 3 — producción

- Secret manager.
- Rate limiting y protección CSRF.
- Manejo de cuentas duplicadas y recuperación.
- Monitoreo de acceso y alertas.

## 12. Criterios de cierre del módulo OAuth2

Se considera cerrado cuando:

1. Los proveedores aparecen cuando tienen credenciales configuradas en el backend.
2. El backend valida state, PKCE, callback fijado y el canje vinculado al navegador.
3. El registro nuevo queda sin permisos hasta aprobación administrativa.
4. Los usuarios aprobados reciben la misma sesión JWT/RBAC local.
5. Existen pruebas unitarias para PKCE, allowlist, state/callback, cookie y configuración.
6. El login real con cada proveedor debe validarse tras configurar credenciales válidas.
7. No hay client secrets en Git ni en el código fuente.

## 13. Conclusión

CIMA cuenta con autenticación local y la base de autenticación federada Google/Microsoft integrada con JWT y RBAC. El siguiente bloque funcional es facturación, seguido de documentos y auditoría. La validación interactiva OAuth queda pendiente de registrar credenciales en cada proveedor y mantenerlas en `.env` o en un gestor de secretos.
