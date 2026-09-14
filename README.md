# CIMA

## Control Interno y Monitoreo de Adultos Mayores

CIMA es una aplicación web interna para apoyar la gestión operativa de una residencia de adultos mayores.

Centraliza residentes, habitaciones, alimentación, indicaciones médicas, medicamentos, estadías, cargos, pagos y auditoría.

> **Proyecto independiente:** CIMA no está asociado, afiliado ni desarrollado para terceros que utilicen nombres comerciales similares.

## Stack

- Go + Chi + MongoDB Driver
- React + TypeScript + Vite
- MongoDB
- JWT + refresh token + Argon2id
- Zod + go-playground/validator
- Vitest + Playwright + Testify
- Docker + Docker Compose
- GitHub Actions

## Inicio rápido con Docker

Requisitos: Docker Desktop y Git. No es necesario instalar Go, Node.js ni MongoDB directamente si se usa Docker.

```bash
docker --version
docker compose version
git --version
```

Clonar:

```bash
git clone <URL_DEL_REPOSITORIO>
cd cima
```

Crear entorno:

### macOS / Linux
```bash
cp .env.example .env
```

### Windows PowerShell
```powershell
Copy-Item .env.example .env
```

Levantar:

```bash
docker compose up --build
```

Servicios:

- Frontend: `http://localhost:5173`
- API: `http://localhost:8080`
- Health: `http://localhost:8080/health`
- MongoDB: `localhost:27017`

Detener:

```bash
docker compose down
```

Volver a levantar:

```bash
docker compose up
```

> No uses `docker compose down -v` salvo que quieras eliminar también los volúmenes y datos locales de MongoDB.

## Comandos útiles

```bash
docker compose ps
docker compose logs -f
docker compose logs -f backend
docker compose logs -f frontend
docker compose build --no-cache
```

## Variables de entorno

```env
APP_ENV=development
APP_PORT=8080
MONGO_URI=mongodb://mongodb:27017
MONGO_DATABASE=cima
JWT_SECRET=change-me
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=168h
CORS_ORIGIN=http://localhost:5173
FILE_STORAGE_PATH=/data/uploads
LOG_LEVEL=debug
```

**Nunca subas `.env` al repositorio.** En producción, usa un gestor de secretos y un `JWT_SECRET` fuerte y aleatorio.

## Estructura

```text
cima/
├── backend/
├── frontend/
├── docker/
├── docs/
├── docker-compose.yml
├── .env.example
├── .gitignore
├── Makefile
└── README.md
```

## Funcionalidades

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

## Seguridad

- Contraseñas con Argon2id.
- Validación crítica también en backend.
- Consultas MongoDB construidas explícitamente.
- No aceptar filtros BSON arbitrarios desde el cliente.
- Documentos médicos en almacenamiento privado.
- No secretos en frontend ni Git.
- Autenticación y autorización por permisos.
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

## API

Base: `/api/v1`

```text
POST   /auth/login
POST   /auth/refresh
POST   /auth/logout
GET    /auth/me

GET    /residents
POST   /residents
GET    /residents/:id
PATCH  /residents/:id
DELETE /residents/:id

GET    /rooms
POST   /rooms
POST   /rooms/:id/assign
POST   /rooms/:id/release

GET    /medication-events
POST   /medication-events/:id/administer

GET    /dashboard
GET    /dashboard/alerts
```

## Roles

- `admin`
- `manager`
- `nurse`
- `kitchen`
- `reception`
- `accounting`

Se recomienda evolucionar a permisos granulares:

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

```bash
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

- **v0.1.0 Foundation:** Docker Compose, MongoDB, Go API, React/TypeScript, health, auth, RBAC, residentes, habitaciones, validación, responsive y tests.
- **v0.2.0 Medical**
- **v0.3.0 Medication**
- **v0.4.0 Billing**
- **v0.5.0 Audit & Security**
- **v1.0.0 Production MVP**

## Privacidad

CIMA puede procesar información personal y de salud. Antes de producción en Chile debe realizarse una revisión legal/compliance de la normativa aplicable, tratamiento de datos de salud, conservación, acceso, respaldos, trazabilidad y gestión de incidentes.

## Contribución

```bash
git checkout -b feature/nombre-feature
```

Implementa, prueba, revisa seguridad/accesibilidad y abre un Pull Request.

## Licencia

Definir antes de publicar el repositorio.

---

**CIMA — Control Interno y Monitoreo de Adultos Mayores**
