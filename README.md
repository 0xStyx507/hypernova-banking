# Hypernova Banking

Plataforma bancaria simplificada y demostrable con Go, chi, PostgreSQL, TigerBeetle, React/Vite y Expo.

## Estado del proyecto

La base funcional incluye migraciones PostgreSQL, seed de usuarios, passwords
hasheadas, sesiones opacas y operaciones financieras respaldadas por
TigerBeetle.

La fuente de verdad financiera será TigerBeetle. PostgreSQL se reservará para identidad, sesiones, auditoría, metadatos y acciones preparadas de MCP.

El registro provisiona una cuenta USD de checking. Los depósitos son una
capacidad de demostración local controlada por `LEDGER_ALLOW_DEMO_DEPOSITS`;
la opción está desactivada por defecto y debe habilitarse explícitamente solo
para una demo local;
una integración real debe reemplazarla por un flujo autorizado de fondeo.

Los datos de prueba y artefactos de evaluación se mantienen fuera del control
de versiones. El seed valida duplicados antes de persistir identidades y no
incluye credenciales en reportes.

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

## Ejecutar la plataforma

```powershell
docker compose build
.\scripts\init-tigerbeetle.ps1
docker compose up
```

La API aplica las migraciones PostgreSQL al iniciar. Para cargar los usuarios
del fixture de desarrollo, ejecuta:

```powershell
.\scripts\seed-users.ps1
```

El seed bloquea por defecto cualquier email duplicado. Para generar un reporte
local de reconciliación sin conectarse a PostgreSQL:

```powershell
New-Item -ItemType Directory -Force data | Out-Null
Push-Location api
go run ./cmd/seed -file ..\datos-prueba-USD.json -duplicates-report ..\data\duplicate-email-report.json -report-only
Pop-Location
```

El reporte solo contiene posiciones de registros y comparaciones booleanas; no
incluye emails, contraseñas, tokens ni números de cuenta.

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

El cliente móvil usa Expo SecureStore para mantener la sesión y consume el
mismo contrato de autenticación, cuentas, saldos, historial y operaciones
idempotentes. Para ejecutarlo:

```powershell
cd mobile
npm install
npm run typecheck
npm run start
```

Endpoints de autenticación:

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout` con `Authorization: Bearer <access_token>`
- `GET /api/v1/auth/mfa` con `Authorization: Bearer <access_token>`
- `POST /api/v1/auth/mfa/enroll` devuelve una URI `otpauth://` para el QR
- `POST /api/v1/auth/mfa/verify` activa el TOTP con un código de seis dígitos

El MFA usa el perfil TOTP estándar de 30 segundos, SHA-1 y seis dígitos. La URI
de enrolamiento es compatible con Google Authenticator y Microsoft
Authenticator; el secreto se cifra antes de guardarse en PostgreSQL y solo se
devuelve durante el enrolamiento.

Endpoints financieros:

- `POST /api/v1/accounts` y `GET /api/v1/accounts`
- `GET /api/v1/accounts/{account_id}`
- `GET /api/v1/accounts/{account_id}/balance`
- `GET /api/v1/accounts/{account_id}/transactions`
- `GET /api/v1/accounts/{account_id}/transactions.csv` (máximo 100 movimientos)
- `POST /api/v1/accounts/{account_id}/deposits`
- `POST /api/v1/accounts/{account_id}/withdrawals`
- `POST /api/v1/transfers`

Las mutaciones financieras requieren `Authorization: Bearer <access_token>` e
`Idempotency-Key`. Los importes se envían como cadenas de unidades menores;
actualmente solo se admite USD.

La superficie autenticada de MCP expone herramientas de lectura y acciones
financieras con el flujo `prepare → confirm` o `cancel`. Preparar no modifica
el ledger; confirmar exige el cuerpo exacto `{ "pin": "1234" }` y
usa el ID preparado como clave idempotente.

El chat autenticado (`POST /api/v1/chat/messages`) usa un proveedor local
determinista y un cliente MCP HTTP. Puede consultar saldo mediante herramientas
de solo lectura; nunca ejecuta una mutación directamente desde el mensaje.

El contrato OpenAPI se encuentra en [`openapi.yaml`](openapi.yaml).

Las respuestas incluyen `X-Request-ID` para correlacionar incidentes y soporte. El cliente puede enviar un UUID propio; si no lo hace, la API genera uno.

El API aplica un límite local configurable con `API_RATE_LIMIT_PER_MINUTE` y
la web permite descargar el historial reciente en CSV.

El baseline de seguridad y riesgos está en
[`docs/compliance/iso-27001-baseline.md`](docs/compliance/iso-27001-baseline.md).

El modelo financiero y sus invariantes están documentados en
[`docs/ledger.md`](docs/ledger.md). PostgreSQL no es la fuente de verdad de
saldos.

## Contratos y organización

El fixture versionado `datos-prueba.json` se valida e importa de forma
idempotente en las tablas de staging. El staging conserva cuentas y movimientos
de origen sin alterar saldos: TigerBeetle sigue siendo la única fuente de
verdad financiera.

El contrato MCP v2, sus herramientas de búsqueda y resúmenes están documentados
en [`docs/mcp.md`](docs/mcp.md). Las reglas de módulos, dependencias y pruebas
DDD están en [`docs/ddd.md`](docs/ddd.md).

## Decisiones

- Monolito modular; no se agregan microservicios, colas, Redis, Kubernetes ni Terraform.
- Los importes se representan con enteros de unidades menores en TigerBeetle; no se usan números de punto flotante.
- La imagen local de TigerBeetle se construye desde un binario oficial fijado para poder formatear el volumen automáticamente.
- `docker compose` orquesta los servicios obligatorios; mobile se ejecuta con Expo fuera de Compose.
