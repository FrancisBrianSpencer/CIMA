SENIOR SUITE
README — Desarrollo local
========================================

Descripción
-----------
Senior Suite es un sistema interno para gestionar residentes de una residencia
de adultos mayores. El stack inicial es Go + React + TypeScript + MongoDB,
ejecutado mediante Docker Compose.

REQUISITOS
----------
- Git
- Docker
- Docker Compose v2
- (Opcional) Go 1.24+ para desarrollo del backend fuera de Docker
- (Opcional) Node.js 22+ y npm para desarrollo del frontend fuera de Docker

ESTRUCTURA
----------
backend/       API REST en Go
frontend/      React + TypeScript
docs/          documentación técnica
docker/        configuración de infraestructura
docker-compose.yml
.env.example

PRIMER ARRANQUE
---------------
1. Clonar el repositorio:

   git clone <URL_DEL_REPOSITORIO>
   cd senior-suite

2. Crear variables de entorno:

   cp .env.example .env

3. Revisar .env. Para desarrollo local puede utilizarse la configuración
   incluida en .env.example.

4. Levantar los servicios:

   docker compose up --build

5. Verificar:

   Frontend:
   http://localhost:5173

   API:
   http://localhost:8080

   Health check:
   http://localhost:8080/health

   MongoDB:
   localhost:27017

Para detener:

   docker compose down

Para detener y eliminar los volúmenes de desarrollo:

   docker compose down -v

NOTA: "down -v" elimina los datos de MongoDB del entorno local.

DESARROLLO SIN DOCKER
---------------------
Backend:

   cd backend
   go mod download
   go run ./cmd/server

Frontend:

   cd frontend
   npm install
   npm run dev

Si el backend corre directamente en el host, ajustar MONGO_URI y
CORS_ORIGIN en .env según corresponda.

COMANDOS RECOMENDADOS
---------------------
Si se incluye Makefile:

   make dev
   make test
   make test-backend
   make test-frontend
   make lint
   make format
   make build
   make docker-up
   make docker-down

BASE DE DATOS
-------------
MongoDB de desarrollo:

   database: senior_suite
   host: localhost
   port: 27017

La aplicación debe ser la única responsable de conectarse a MongoDB.
El frontend nunca debe conectarse directamente a la base de datos.

DATOS DE DESARROLLO
-------------------
Para desarrollo, utilizar únicamente datos ficticios.

Si el proyecto incorpora un seed:

   make seed

No introducir nombres, RUT, diagnósticos, recetas, teléfonos ni otros datos
reales en fixtures, tests, logs o commits.

AUTENTICACIÓN
-------------
La aplicación utilizará autenticación con access token y refresh token.

Nunca:
- guardar secretos en el frontend;
- guardar contraseñas en texto plano;
- subir .env al repositorio;
- guardar JWT_SECRET en el código fuente;
- almacenar información médica en localStorage.

SEGURIDAD
---------
El backend debe validar nuevamente todos los datos recibidos por HTTP.

No confiar en:
- validaciones del navegador;
- permisos enviados por React;
- IDs proporcionados por el cliente;
- objetos BSON arbitrarios enviados por el cliente.

Para MongoDB se debe prevenir NoSQL injection mediante DTOs, validación
estricta y construcción explícita de filtros.

La aplicación no intenta bloquear F12. Todo secreto y toda autorización
deben estar protegidos en el servidor.

ARCHIVOS MÉDICOS
----------------
Los documentos médicos no deben publicarse directamente bajo el directorio
web.

La aplicación debe almacenar metadata en MongoDB y los archivos en un
storage privado.

ACCESIBILIDAD
-------------
La UI debe ser mobile-first y soportar:
- navegación por teclado;
- foco visible;
- modo oscuro;
- zoom nativo del navegador;
- contraste adecuado;
- mensajes de error accesibles;
- no depender únicamente del color.

TESTS
-----
Antes de abrir un Pull Request:

   go test ./...

y, desde frontend:

   npm test

Los flujos E2E deben ejecutarse con Playwright cuando estén configurados.

FLUJO DE TRABAJO GIT
--------------------
Crear una rama por feature:

   git checkout -b feat/resident-crud

Commit:

   git add .
   git commit -m "feat: implement resident CRUD"

Luego abrir Pull Request.

No hacer commits directamente sobre main salvo que el flujo del repositorio
lo permita explícitamente.

VARIABLES DE ENTORNO
--------------------
Ejemplo:

APP_ENV=development
APP_PORT=8080
MONGO_URI=mongodb://mongodb:27017
MONGO_DATABASE=senior_suite
JWT_SECRET=change-me
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=168h
CORS_ORIGIN=http://localhost:5173
FILE_STORAGE_PATH=/data/uploads
LOG_LEVEL=debug

En producción, JWT_SECRET y cualquier credencial deben provenir de un
secret manager o mecanismo equivalente.

TROUBLESHOOTING
---------------
Si Docker no levanta:

   docker compose ps
   docker compose logs

Logs del backend:

   docker compose logs backend

Logs de MongoDB:

   docker compose logs mongodb

Si hay problemas de puertos, verificar que 5173, 8080 y 27017 estén libres.

Si se cambió la configuración de dependencias o Dockerfiles:

   docker compose build --no-cache
   docker compose up

LICENCIA
--------
Definir la licencia del proyecto antes de publicar el repositorio.

DOCUMENTACIÓN
-------------
La especificación técnica completa se encuentra en:

   docs/architecture.md
   docs/api.md
   docs/security.md
   docs/database.md
   docs/accessibility.md

También se debe conservar en el repositorio la versión PDF de la
Especificación Técnica.

PRIMER MILESTONE
----------------
v0.1.0 — Foundation

Objetivo:

- Docker Compose
- MongoDB
- API Go
- React + TypeScript
- health check
- autenticación
- RBAC
- layout responsive
- CRUD de residentes
- CRUD de habitaciones
- validación
- tests básicos
- documentación inicial
