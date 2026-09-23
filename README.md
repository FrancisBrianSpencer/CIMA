# CIMA v0.1.0

## Control Interno y Monitoreo de Adultos Mayores

Base inicial de CIMA: API Go, frontend React y MongoDB ejecutados mediante Docker Compose.

> Proyecto independiente, sin asociación con terceros que utilicen nombres comerciales similares.

## Requisitos

- Docker Desktop
- Git

## Ejecutar

```bash
cp .env.example .env
docker compose up --build
```

Windows PowerShell:

```powershell
Copy-Item .env.example .env
docker compose up --build
```

- Frontend: http://localhost:5173
- API: http://localhost:8080
- Health: http://localhost:8080/health
- Residentes: http://localhost:8080/api/v1/residents

## Comandos

```bash
docker compose ps
docker compose logs -f
docker compose down
```

No utilices `docker compose down -v` salvo que quieras borrar los datos locales de MongoDB.

## Estado de esta versión

Incluye un health check, conexión a MongoDB y un endpoint inicial para listar/crear residentes. Es una base de desarrollo; todavía no debe utilizarse con datos reales de salud.
