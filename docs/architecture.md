# Arquitectura inicial

Este documento resume las decisiones de la plataforma y el estado comprobado
del repositorio. La plataforma combina identidad en PostgreSQL con un ledger
financiero respaldado por TigerBeetle.

## Estado de Fase 0

| Área | Estado | Evidencia |
| --- | --- | --- |
| Monorepo | Implementado y validado | Directorios `api`, `web`, `mobile`, `docker`, `scripts` |
| API Go + chi | Implementado y validado | `go test ./...`, build Docker y healthcheck |
| PostgreSQL | Implementado y validado | Migración inicial, usuarios, sesiones y auditoría |
| TigerBeetle | Implementado y validado | Servicio saludable y volumen de desarrollo inicializado |
| Web React/Vite/TS/Tailwind | Implementado y validado | lint, build local y build Docker |
| Mobile Expo | Implementado, pendiente de typecheck | `mobile/app` y `mobile/package.json` |
| Health checks | Implementado y validado | `/healthz`, `/readyz` verifica PostgreSQL y TigerBeetle, y healthchecks Compose |

## Límites de responsabilidad

- PostgreSQL: usuarios, autenticación, sesiones, auditoría y metadatos.
- TigerBeetle: cuentas, transferencias y balances; será la única fuente de verdad financiera.
- API: validación, autorización, casos de uso y contratos HTTP.
- Web y mobile: presentación y consumo de contratos; nunca deciden reglas financieras.
- IA/MCP: usa herramientas autenticadas y una frontera persistida
  `prepare/confirm/cancel`; ninguna acción financiera se ejecuta desde texto
  libre sin confirmación explícita.

## Validación de fase 0

La ejecución local validada produjo cuatro contenedores saludables: API,
PostgreSQL, TigerBeetle y web. La configuración de Compose también fue
validada con `docker compose config`. Los endpoints `/healthz`, `/readyz` y la
web respondieron HTTP 200.

## Riesgos y pendientes de fase 0

- El typecheck de mobile todavía no se ha ejecutado porque sus dependencias
  locales no están instaladas.
- El cliente nativo de TigerBeetle requiere `seccomp=unconfined` e `IPC_LOCK` en este entorno Docker local; ambos quedan explícitos en Compose y deben endurecerse antes de producción.
- El binario de TigerBeetle está versionado por variable de entorno; el checksum se añadirá en la fase de hardening.

## Identidad y ledger

La identidad usa una migración embebida, seed de usuarios y sesiones opacas con
rotación de refresh tokens. Las cuentas HNL se provisionan en TigerBeetle y se
relacionan con el usuario en PostgreSQL. Los movimientos utilizan claves de
idempotencia, estados `unknown` para respuestas inciertas y el mismo
identificador de transferencia cuando se reintentan.

La superficie HTTP versionada de esta fase se mantiene en
[`openapi.yaml`](openapi.yaml), que es el contrato compartido por API, web y
mobile y clientes MCP.

El baseline de seguridad y riesgos se documenta en
[`compliance/iso-27001-baseline.md`](compliance/iso-27001-baseline.md), con el
alcance mínimo necesario para la prueba técnica.

El seed está bloqueado actualmente por 20 registros con emails duplicados en
`datos-prueba-HNL.json`. El comando falla antes de abrir la transacción para no
seleccionar silenciosamente una identidad ni dejar datos parciales. El comando
también puede generar un reporte local de reconciliación con posiciones de
registros y comparaciones booleanas, sin incluir emails, contraseñas, tokens ni
números de cuenta.

El detalle del modelo financiero está en [`ledger.md`](ledger.md).
