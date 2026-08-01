# Arquitectura inicial

Este documento resume las decisiones de la plataforma y el estado comprobado
del repositorio. La fase actual es una base ejecutable; la persistencia de
identidad y la migración del fixture se implementarán en fase 1.

## Estado de Fase 0

| Área | Estado | Evidencia |
| --- | --- | --- |
| Monorepo | Implementado y validado | Directorios `api`, `web`, `mobile`, `docker`, `scripts` |
| API Go + chi | Implementado y validado | `go test ./...`, build Docker y healthcheck |
| PostgreSQL | Implementado y validado | Servicio saludable en Compose; sin tablas todavía |
| TigerBeetle | Implementado y validado | Servicio saludable y volumen de desarrollo inicializado |
| Web React/Vite/TS/Tailwind | Implementado y validado | lint, build local y build Docker |
| Mobile Expo | Implementado, pendiente de typecheck | `mobile/app` y `mobile/package.json` |
| Health checks | Implementado y validado | `/healthz`, `/readyz` y healthchecks Compose |

## Límites de responsabilidad

- PostgreSQL: usuarios, autenticación, sesiones, auditoría y metadatos.
- TigerBeetle: cuentas, transferencias y balances; será la única fuente de verdad financiera.
- API: validación, autorización, casos de uso y contratos HTTP.
- Web y mobile: presentación y consumo de contratos; nunca deciden reglas financieras.
- IA/MCP: se incorporarán después con prepare/confirm/cancel en servidor.

## Validación de fase 0

La ejecución local validada produjo cuatro contenedores saludables: API,
PostgreSQL, TigerBeetle y web. La configuración de Compose también fue
validada con `docker compose config`. Los endpoints `/healthz`, `/readyz` y la
web respondieron HTTP 200.

## Riesgos y pendientes de fase 0

- El typecheck de mobile todavía no se ha ejecutado porque sus dependencias
  locales no están instaladas.
- TigerBeetle requiere memoria y `seccomp=unconfined` en Docker en algunos entornos; Compose deja esta configuración explícita.
- El binario de TigerBeetle está versionado por variable de entorno; el checksum se añadirá en la fase de hardening.

## Fase 1 todavía no iniciada

No existen aún migraciones, tablas PostgreSQL, repositorios ni comando de seed.
El contrato del fixture y las reglas de importación están documentados en
[`phase-1-data-migration.md`](phase-1-data-migration.md).
