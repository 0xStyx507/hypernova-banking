# Baseline de seguridad — ISO/IEC 27001:2022

## Alcance

Este baseline cubre la API Go, PostgreSQL, TigerBeetle, los contratos HTTP,
los procesos de migración y los clientes web/mobile de Hypernova Banking.
Su propósito es aplicar seguridad por diseño durante el desarrollo. No es una
declaración de certificación ni sustituye una auditoría independiente.

La referencia adoptada es ISO/IEC 27001:2022 con su enmienda 1:2024. La norma
define requisitos para un sistema de gestión de seguridad de la información;
la selección final de controles debe depender del contexto, riesgos y alcance
de la organización.

## Objetivos de seguridad

| Objetivo | Aplicación en Hypernova |
| --- | --- |
| Confidencialidad | No registrar contraseñas ni tokens; proteger datos personales y secretos de configuración. |
| Integridad | TigerBeetle como fuente financiera; restricciones únicas, migraciones transaccionales e idempotencia. |
| Disponibilidad | Health checks, readiness, timeouts, recuperación documentada y pruebas de fallo. |
| Trazabilidad | Auditoría de autenticación, sesiones, migraciones y eventos de seguridad sin secretos. |
| Privacidad | Minimización, clasificación, finalidad y retención controlada de datos personales. |

## Registro inicial de riesgos

| ID | Riesgo | Impacto | Tratamiento inicial | Evidencia |
| --- | --- | --- | --- | --- |
| R-001 | Emails duplicados pueden asociar una identidad incorrecta. | Alto | Fallar antes de insertar, no fusionar automáticamente y generar reporte seguro. | `api/internal/seed`, pruebas de seed |
| R-002 | Exposición de contraseñas del fixture. | Crítico | Fixture local, `.gitignore`, bcrypt durante seed y prohibición de secretos en logs. | `api/internal/auth`, `.gitignore` |
| R-003 | Robo o reutilización de sesiones. | Alto | Tokens opacos, hashes SHA-256, expiración, rotación de refresh y revocación. | `api/internal/auth`, `sessions` |
| R-004 | Cambios de esquema incompletos o no repetibles. | Alto | Migraciones numeradas, registradas y aplicadas dentro de transacciones. | `api/internal/db` |
| R-005 | Auditoría puede revelar información sensible. | Alto | Eventos con metadatos mínimos y exclusión explícita de credenciales. | `api/internal/audit` |
| R-006 | Un error de integración podría alterar el saldo financiero. | Crítico | PostgreSQL no almacena la verdad financiera; TigerBeetle será el ledger único. | `docs/architecture.md` |

## Controles aplicados

- Validación de entrada y rechazo de campos JSON desconocidos.
- Contrato OpenAPI para la superficie HTTP.
- Contraseñas con bcrypt y límite compatible con el algoritmo.
- Access y refresh tokens aleatorios, opacos y almacenados solo como hashes.
- Rotación transaccional de refresh tokens y revocación idempotente.
- Auditoría de registro, login, fallos de login, refresh, logout y seed.
- Restricción de email único en PostgreSQL y preflight del fixture.
- Migraciones embebidas, ordenadas y registradas.
- Pruebas unitarias, `go vet`, smoke tests HTTP, validación de Compose y CI.

## Riesgos residuales y mejoras

Estos controles requieren una decisión operativa, infraestructura adicional o
un alcance regulatorio específico:

- MFA, recuperación de cuenta y bloqueo adaptativo; existe un rate limit local
  configurable, pero una instalación distribuida debe moverlo al gateway.
- Gestión externa de secretos y rotación de claves.
- TLS terminado en infraestructura, cabeceras de seguridad y gestión de CORS.
- Roles administrativos, mínimo privilegio y separación de funciones.
- Retención, acceso y exportación de auditoría con protección contra alteración.
- Respuesta a incidentes, continuidad, recuperación y pruebas de restauración.
- SAST/DAST, escaneo de dependencias, revisión de imágenes y pruebas externas.
- Evaluación de terceros y proveedores cloud.
- KYC/AML, monitoreo transaccional y reportes regulatorios cuando el producto
  opere como servicio financiero supervisado.

## Evidencias y criterio de cierre

Cada control debe tener una referencia a código, configuración, prueba,
registro o procedimiento. Un control se considera cerrado únicamente cuando:

1. existe una implementación o procedimiento aprobado;
2. existe una prueba o evidencia reproducible;
3. el riesgo residual tiene responsable y decisión documentada.

El cumplimiento regulatorio de una entidad bancaria panameña debe validarse
contra su licencia, productos, terceros y obligaciones vigentes con asesoría
legal y de cumplimiento.
