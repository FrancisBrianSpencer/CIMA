# CIMA

## Control Interno y Monitoreo de Adultos Mayores

CIMA es una aplicación web interna para apoyar la gestión operativa de una residencia de adultos mayores.

Centraliza residentes, habitaciones, alimentación, indicaciones médicas, medicamentos, estadías, cargos, pagos y auditoría.

> **Proyecto independiente:** CIMA no está asociado, afiliado ni desarrollado para terceros que utilicen nombres comerciales similares.

## v0.1.1 — Foundation / Docker-only

Esta versión corrige el flujo de construcción del backend para que **Go no tenga que estar instalado en Windows**. Go se utiliza dentro del contenedor de compilación de Docker, y Node.js se utiliza dentro del contenedor del frontend.

El entorno local requiere únicamente Docker Desktop y Git. No es necesario instalar directamente Go, Node.js ni MongoDB.

## Stack

- Go + Chi + MongoDB Driver
- React + TypeScript + Vite
- MongoDB
- Docker + Docker Compose
- Preparado para JWT + refresh token + Argon2id
- Preparado para Zod + go-playground/validator
- Preparado para Vitest + Playwright + Testify
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
```

## Variables de entorno

```env
APP_ENV=development
APP_PORT=8080
MONGO_URI=mongodb://mongodb:27017
MONGO_DATABASE=cima
JWT_SECRET=change-this-development-secret
CORS_ORIGIN=http://localhost:5173
```

**Nunca subas `.env` al repositorio.** En producción, usa un gestor de secretos y un `JWT_SECRET` fuerte y aleatorio.

## Estructura actual

```text
CIMA/
├── backend/
│   ├── cmd/server/main.go
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
├── .env.example
├── .gitignore
├── Makefile
└── README.md
```

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

## Estado funcional de v0.1.1

La versión actual es una **base técnica inicial**, no todavía el MVP completo.

Incluye:
- API Go funcionando
- Conexión con MongoDB
- Endpoint `/health`
- Listado y creación inicial de residentes
- Frontend React/TypeScript
- Formulario inicial de residentes
- Responsive inicial
- Modo oscuro basado en preferencias del sistema
- Validación básica de campos requeridos
- Construcción completa mediante Docker

Autenticación, RBAC, módulos médicos, medicamentos, facturación, auditoría avanzada y pruebas E2E quedan para iteraciones posteriores.

## Seguridad

Objetivos:
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

## API actual

Base: `/api/v1`

Actualmente implementado:
```text
GET    /health
GET    /api/v1/residents
POST   /api/v1/residents
```

API prevista:
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
- **v0.2.0 Medical**
- **v0.3.0 Medication**
- **v0.4.0 Billing**
- **v0.5.0 Audit & Security**
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
