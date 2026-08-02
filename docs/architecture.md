# Arquitectura

Hypernova Banking es un monolito modular que combina identidad en PostgreSQL,
contabilidad financiera en TigerBeetle y clientes web/mobile que consumen un
contrato HTTP común.

## Diseño de dominio

El sistema aplica DDD de forma incremental dentro de un único proceso. Cada
módulo es un contexto delimitado con reglas y casos de uso propios; no se
comparten entidades mutables entre módulos ni se convierten las rutas HTTP en
la lógica del negocio.

| Contexto | Código | Responsabilidad principal |
| --- | --- | --- |
| Identidad y acceso | `api/internal/auth` | Usuarios, sesiones, tokens y MFA |
| Cuentas y ledger | `api/internal/ledger` | Ownership, saldos y movimientos en TigerBeetle |
| Aprobaciones | `api/internal/mcp` | Intención persistida, confirmación, expiración e idempotencia |
| Asistente | `api/internal/assistant` | Conversación y acceso controlado a herramientas |
| Auditoría | `api/internal/audit` | Evidencia de eventos de seguridad y operaciones |
| Infraestructura | `api/internal/db` | PostgreSQL, migraciones y conexión de persistencia |

`api/cmd/server` funciona como adaptador de entrada: decodifica HTTP, aplica
guards y traduce errores a contratos públicos. Los servicios internos son la
capa de aplicación y dominio; PostgreSQL y TigerBeetle son adaptadores de
infraestructura. React y Expo solo presentan el contrato, por lo que nunca
deciden autorización, ownership, saldo o reglas monetarias.

Las unidades menores se mantienen como enteros decimales y los cambios
financieros pasan por el caso de uso del ledger. Esta separación permite
extraer repositorios o adaptadores en el futuro sin convertir el proyecto en
microservicios ni añadir infraestructura no requerida.

## Responsabilidades

- PostgreSQL: usuarios, sesiones, auditoría, propiedad de cuentas, operaciones
  idempotentes y acciones MCP preparadas.
- TigerBeetle: cuentas, transferencias y balances; es la única fuente de verdad
  financiera.
- API Go + chi: validación, autorización, casos de uso, migraciones y contrato
  HTTP.
- Web React/Vite y mobile Expo: presentación y consumo del contrato; no
  deciden reglas financieras.
- MCP y asistente: herramientas autenticadas, proveedor local desacoplado y
  frontera persistida `prepare/confirm/cancel`.

## Flujo financiero

Los importes públicos son cadenas de unidades menores enteras y solo HNL está
habilitado. Toda mutación usa `Idempotency-Key`; el API conserva el mismo ID de
transferencia para reconciliar respuestas inciertas. El cliente no realiza
pre-chequeos de saldo que puedan introducir una carrera: TigerBeetle aplica la
restricción de fondos.

Las acciones solicitadas por MCP se validan, se guardan con un hash de
integridad y expiran. Solo una confirmación exacta puede ejecutar una acción;
la cancelación no puede competir con una acción ya reclamada.

## Disponibilidad y operación

Compose coordina PostgreSQL, TigerBeetle, API y web. `/healthz` confirma el
proceso y `/readyz` confirma las dependencias críticas. El API define timeouts,
rate limiting local, logs estructurados y migraciones embebidas transaccionales.

El cliente nativo de TigerBeetle requiere `seccomp=unconfined` e `IPC_LOCK` en
este entorno Docker local. Una instalación productiva debe revisar ambos
requisitos, aislar la red, terminar TLS en infraestructura y usar un flujo de
fondeo autorizado.

## Contratos y seguridad

La superficie pública se mantiene en [`openapi.yaml`](openapi.yaml), que es
compartida por API, web, mobile y clientes MCP. El baseline mínimo de
seguridad se encuentra en
[`compliance/iso-27001-baseline.md`](compliance/iso-27001-baseline.md).

El seed rechaza emails duplicados antes de abrir la transacción y nunca
selecciona silenciosamente una identidad. Los reportes locales contienen solo
posiciones y comparaciones booleanas, nunca credenciales, tokens o números de
cuenta.

El detalle del ledger está en [`ledger.md`](ledger.md) y el modelo de sesiones
en [`authentication.md`](authentication.md).
