# Hypernova Banking

Plataforma bancaria simplificada y demostrable con Go, chi, PostgreSQL, TigerBeetle, React/Vite y Expo.

## Estado del proyecto

Fase 0 implementada: monorepo base, API mínima, web mínima, mobile mínima, PostgreSQL, TigerBeetle, Docker Compose y health checks.

La fuente de verdad financiera será TigerBeetle. PostgreSQL se reservará para identidad, sesiones, auditoría y metadatos.

El fixture de datos para la fase 1 es `datos-prueba-HNL.json` en la raíz. No se
importa durante la fase 0; las reglas de hash, validación e idempotencia están
documentadas en `docs/phase-1-data-migration.md`.

## Estructura

```text
api/       API Go + chi
web/       Frontend React + Vite + TypeScript + Tailwind
mobile/    Expo + React Native + Expo Router + NativeWind
docker/    Imágenes y bootstrap de infraestructura
scripts/   Operaciones locales repetibles
docs/      Decisiones y alcance técnico
```

## Requisitos locales

- Docker Desktop con Compose v2
- Go 1.24+
- Node.js 22+
- npm 10+

## Configuración

```powershell
Copy-Item .env.example .env
```

Los valores de `.env.example` son exclusivamente para desarrollo local. No registrar secretos reales.

## Ejecutar Fase 0

```powershell
docker compose build
.\scripts\init-tigerbeetle.ps1
docker compose up
```

Endpoints disponibles:

- API: `http://localhost:8080/healthz`
- API readiness: `http://localhost:8080/readyz`
- Web: `http://localhost:4173`
- PostgreSQL: `localhost:5432`
- TigerBeetle: `localhost:3000`

La inicialización de TigerBeetle se ejecuta una sola vez por volumen. El script no sobrescribe un archivo existente.

## Validación local

```powershell
go test ./...
cd web; npm ci; npm run lint; npm run build
cd ..\mobile; npm install; npx tsc --noEmit
docker compose config
```

En este punto la API solo expone health checks. Las migraciones, autenticación, cuentas y operaciones financieras pertenecen a las fases siguientes.

## Decisiones

- Monolito modular; no se agregan microservicios, colas, Redis, Kubernetes ni Terraform.
- No se representa dinero en esta fase; las operaciones monetarias se implementarán con enteros de unidades menores en TigerBeetle.
- La imagen local de TigerBeetle se construye desde el binario oficial versionado para poder formatear el volumen automáticamente.
- `docker compose` orquesta los servicios obligatorios; mobile se ejecuta con Expo fuera de Compose.
