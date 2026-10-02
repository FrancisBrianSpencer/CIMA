# CIMA

## Control Interno y Monitoreo de Adultos Mayores

CIMA es una aplicación web interna para apoyar la gestión operativa de una residencia de adultos mayores.

Centraliza residentes, habitaciones, alimentación, indicaciones médicas, medicamentos, estadías, cargos, pagos y auditoría.

> **Proyecto independiente:** CIMA no está asociado, afiliado ni desarrollado para terceros que utilicen nombres comerciales similares.

## Estado del proyecto — versión técnica actualizada v0.7

La especificación técnica actual está en [docs/ESPECIFICACION_TECNICA_CIMA_v0.6.md](docs/ESPECIFICACION_TECNICA_CIMA_v0.6.md). En ella se reflejan los avances verificados: residentes, habitaciones, estadías, atención clínica, medicamentos, alimentación y autenticación OAuth2 para Google y Microsoft. La autenticación federada requiere registrar credenciales en los proveedores y guardarlas únicamente en `.env`.

La última release/tag publicada sigue siendo v0.3, pero el estado real del proyecto está descrito en el documento v0.6 y no corresponde a un release formal todavía. El entorno requiere únicamente Docker Desktop y Git; no es necesario instalar Go, Node.js ni MongoDB en el equipo.

## Stack

- Go + Chi + MongoDB Driver
- React + TypeScript + Vite
- MongoDB
- Docker + Docker Compose
- JWT HS256, refresh token y bcrypt implementados; RBAC backend por permiso
- Vitest, pruebas Go e integración API/MongoDB implementadas; pruebas E2E pendientes
- Interfaz y mensajes en español, UTF-8 y locale `es-CL`
- GitHub Actions

## Inicio rápido con Docker

### Requisitos

Comprueba:

```powershell
docker --version
docker compose version
git --version
```

### Clonar

```powershell
git clone https://github.com/FrancisBrianSpencer/CIMA.git
cd CIMA
```

### Configurar entorno

Windows PowerShell:

```powershell
Copy-Item .env.example .env
```

macOS / Linux:

```bash
cp .env.example .env
```

### Levantar CIMA

```powershell
docker compose up --build
```

La primera ejecución puede tardar porque Docker descarga las imágenes base y construye los contenedores.

### Servicios

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080`
- Health: `http://localhost:8080/health`
- MongoDB: `localhost:27017`

Para detener:

```powershell
docker compose down
```

Para volver a levantar:

```powershell
docker compose up
```

> No uses `docker compose down -v` salvo que quieras eliminar también el volumen y los datos locales de MongoDB.

## Qué cambió en v0.1.1

El backend utiliza una imagen de compilación Go dentro de Docker. La resolución de dependencias y la compilación ocurren dentro de ese contenedor, por lo que el host no necesita Go.

```text
Windows
  │
  └── Docker Desktop
       ├── frontend
       │    └── Node.js
       ├── backend
       │    └── Go
       └── mongodb
            └── MongoDB
```

## Comandos útiles

```powershell
docker compose ps
docker compose logs -f
docker compose logs -f backend
docker compose logs -f frontend
docker compose build --no-cache
docker compose --profile integration run --rm integration-tests
```

## Variables de entorno

```env
APP_ENV=development
APP_PORT=8080
MONGO_URI=mongodb://mongodb:27017
MONGO_DATABASE=cima
JWT_SECRET=REPLACE_WITH_STRONG_PRIVATE_SECRET
CORS_ORIGIN=http://localhost:5173
OAUTH_GOOGLE_CLIENT_ID=
OAUTH_GOOGLE_CLIENT_SECRET=
OAUTH_GOOGLE_REDIRECT_URI=http://localhost:8080/auth/oauth/google/callback
OAUTH_MICROSOFT_CLIENT_ID=
OAUTH_MICROSOFT_CLIENT_SECRET=
OAUTH_MICROSOFT_TENANT_ID=common
OAUTH_MICROSOFT_REDIRECT_URI=http://localhost:8080/auth/oauth/microsoft/callback
OAUTH_FRONTEND_REDIRECT_URI=http://localhost:5173/
OAUTH_ALLOWED_DOMAINS=
```

**Nunca subas `.env` al repositorio.** En producción, usa un gestor de secretos y un `JWT_SECRET` fuerte y aleatorio.

## Estructura actual

```text
CIMA/
├── backend/
│   ├── cmd/server/main.go
│   ├── cmd/server/main_test.go
│   ├── cmd/server/api_integration_test.go
│   ├── Dockerfile
│   ├── .dockerignore
│   ├── go.mod
│   └── go.sum
├── frontend/
│   ├── src/
│   │   ├── App.tsx
│   │   ├── main.tsx
│   │   └── styles.css
│   ├── Dockerfile
│   ├── .dockerignore
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── docker-compose.yml
├── docs/
│   ├── ESPECIFICACION_TECNICA_CIMA_v0.3.md / .pdf
│   ├── ESPECIFICACION_TECNICA_CIMA_v0.4.md / .pdf
│   └── ESPECIFICACION_TECNICA_CIMA_v0.5.md / .pdf
├── .env.example
├── .gitignore
├── Makefile
└── README.md
```

## OAuth2 y autenticación federada

El backend soporta inicio de sesión y registro con Google y Microsoft/Outlook. Usa `state`, PKCE S256, callbacks fijados por configuración y códigos de canje de un solo uso; después emite los mismos JWT locales con RBAC. Los client secrets permanecen en `.env` y no se envían al frontend.

Configura en Google Cloud Console y Microsoft Entra ID los callbacks exactos indicados en `.env.example`. Al completar esas credenciales, `docker compose up --build` habilita los botones automáticamente. Las cuentas nuevas quedan en rol `pending`, sin permisos, hasta que un administrador les asigne un rol aprobado. El enlace automático por correo está deshabilitado para evitar apropiación de cuentas existentes.

## Funcionalidades previstas

### Residentes
- Datos personales y contactos
- Contacto de emergencia
- Preferencias, alergias y restricciones alimentarias
- Indicaciones médicas
- Habitación
- Estado de residencia

### Habitaciones
- Disponibilidad y ocupación
- Mantenimiento
- Capacidad
- Asignación y liberación

### Medicamentos
- Prescripciones
- Dosis, vía, frecuencia y horarios
- Vigencia
- Eventos individuales de administración
- Estados: pendiente, administrado, omitido, rechazado y no administrado

### Estadías y pagos
- Ingreso/egreso
- Tarifas
- Cargos
- Mantención y servicios
- Pagos
- Estado de cuenta

### Auditoría
Acciones relevantes como login/logout, cambios de residentes, información médica, medicamentos, administración de dosis, documentos, habitaciones, cargos/pagos y usuarios/roles.

## Estado funcional actual

El proyecto ya no está en el punto de una base preliminar: el estado real del código incluye:

- API Go funcionando con MongoDB
- Endpoint `/health`
- CRUD de residentes con archivado lógico
- Habitaciones: catálogo, asignación/liberación, ocupación y trazabilidad protegida por RBAC
- Estadías: modelo, validación y rutas CRUD
- Atención clínica: notas con severidad, observaciones y lista por residente
- Medicación: eventos programados/administrados con registro por residente
- Timestamps de creación, actualización y archivado
- Frontend React/TypeScript con módulos visibles según permisos
- Login, refresh y perfil autenticado
- Gestión de usuarios por API y permisos por endpoint
- Construcción completa mediante Docker
- Pruebas Go y build Vite ejecutados con Docker
- Mensajes en español y compatibilidad de idioma `es-CL`

Lo que falta antes de cerrar la primera versión operativa es principalmente: alimentación, facturación/documentos, auditoría real, administración de usuarios avanzada y la integración OAuth2 con Google/Microsoft, además de hardening de seguridad para producción.

## Seguridad

Implementado:
- bcrypt para contraseñas, JWT de acceso (15 minutos) y refresh (7 días)
- Middleware RBAC backend y restricciones de CORS para origen y encabezados autorizados
- DTOs estrictos en endpoints que decodifican JSON y límite de 1 MiB

Pendiente antes de producción:
- Contraseñas con Argon2id.
- Validación crítica también en backend.
- Consultas MongoDB construidas explícitamente.
- No aceptar filtros BSON arbitrarios desde el cliente.
- Documentos médicos en almacenamiento privado.
- No secretos en frontend ni Git.
- Revocación/logout de refresh tokens, rotación y reducción de riesgos por permisos obsoletos en JWT.
- Secreto fuerte obligatorio por ambiente; retirar el secreto fallback y cambiar credenciales semilla.
- Auditoría real de accesos y cambios sensibles.
- HTTPS, CORS restrictivo, rate limiting y cabeceras de seguridad en producción.
- No registrar información médica sensible.

### F12 / DevTools

CIMA no intentará impedir F12, DevTools ni la inspección del frontend. Todo lo enviado al navegador puede ser inspeccionado. La seguridad debe depender del backend y de los controles de acceso.

## Accesibilidad

Objetivo **WCAG 2.2 AA**:
- Navegación por teclado
- Foco visible
- HTML semántico
- Labels y errores accesibles
- Contraste adecuado
- No depender solo del color
- Modo claro/oscuro/sistema
- `prefers-reduced-motion`
- Touch targets adecuados
- Zoom nativo del navegador, procurando mantener la funcionalidad al 200%

## API actual

Base: `/api/v1`

Actualmente implementado (las rutas de negocio indicadas requieren bearer token y permiso):
```text
GET    /health
POST   /auth/login
POST   /auth/refresh
GET    /auth/me
GET/POST/PATCH  /api/v1/users
GET    /api/v1/residents
POST   /api/v1/residents
GET    /api/v1/residents/{id}
PATCH  /api/v1/residents/{id}
DELETE /api/v1/residents/{id}                 # archivado lógico
GET    /api/v1/residents/{id}/profile
PATCH  /api/v1/residents/{id}/profile
GET    /api/v1/rooms
POST   /api/v1/rooms
GET    /api/v1/rooms/{id}
PATCH  /api/v1/rooms/{id}
POST   /api/v1/rooms/{id}/assign
POST   /api/v1/rooms/{id}/release
GET    /api/v1/rooms/{id}/history
GET    /api/v1/dashboard                      # respuesta de estado inicial
GET    /api/v1/audit                           # respuesta de estado inicial
GET/POST /api/v1/documents                    # lectura placeholder; escritura HTTP 501
GET/POST /api/v1/billing                      # lectura placeholder; escritura HTTP 501
GET/POST /api/v1/medical                      # lectura placeholder; escritura HTTP 501
GET/POST /api/v1/medication-events            # lectura placeholder; escritura HTTP 501
```

Las operaciones de usuarios requieren `user.read` o `user.write`; las demás usan permisos como `resident.read`, `room.write`, `medical.read`, `billing.read`, `document.read`, `medication.read`, `audit.read` y `dashboard.read`. Asignar/liberar habitaciones requiere `room.write` y `resident.read`. El rol admin omite la comprobación granular. Las rutas placeholder no representan módulos funcionales.

Pendiente en API:
```text
POST   /auth/logout
POST   /medication-events/:id/administer
GET    /dashboard/alerts
```

## Roles previstos

- `admin`
- `manager`
- `nurse`
- `kitchen`
- `reception`
- `accounting`

Permisos granulares previstos:
```text
resident.read
resident.create
medical.read
medical.update
medication.administer
billing.read
billing.create
audit.read
```

## Testing

Previsto:
```powershell
make test
make test-backend
make test-frontend
make lint
```

E2E mínimo:
1. Login
2. Crear residente
3. Asignar habitación
4. Crear medicamento
5. Administrar dosis
6. Registrar pago

## Roadmap

- **v0.1.0 Foundation:** Docker Compose, MongoDB, Go API, React/TypeScript, health y base de residentes.
- **v0.1.1 Docker-only Foundation:** build del backend dentro de Docker y README introductorio actualizado.
- **v0.2.0 Authentication & roles:** login JWT, refresh token, usuario administrador inicial y perfil autenticado.
- **v0.3.0 Medical**
- **v0.4.0 Medication**
- **v0.5.0 Billing**
- **v0.6.0 Audit & Security**
- **v1.0.0 Production MVP**

## Privacidad

CIMA puede procesar información personal y de salud. Antes de producción en Chile debe realizarse una revisión legal/compliance de la normativa aplicable, tratamiento de datos de salud, conservación, acceso, respaldos, trazabilidad y gestión de incidentes.

## Contribución

```powershell
git checkout -b feature/nombre-feature
```

Implementa, prueba, revisa seguridad/accesibilidad y abre un Pull Request.

## Licencia

Definir antes de publicar el repositorio.

---

**CIMA — Control Interno y Monitoreo de Adultos Mayores**
