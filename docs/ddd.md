# Organización modular y DDD

El repositorio es un monolito modular. Cada contexto tiene su propio modelo,
casos de uso, validaciones y pruebas; compartir utilidades pequeñas no autoriza
a compartir entidades mutables.

```text
api/cmd/server/       adaptadores HTTP, autenticación de rutas y serialización
api/internal/auth/    identidad, sesiones, MFA y PIN
api/internal/ledger/  cuentas, ownership, consultas y TigerBeetle
api/internal/mcp/     acciones preparadas, confirmación e idempotencia
api/internal/assistant/ lenguaje, intención y selección de herramientas
api/internal/seed/    validación e importación de fixtures
api/internal/audit/   eventos de seguridad y negocio
api/internal/db/      conexión y migraciones PostgreSQL
```

## Regla de dependencias

- `cmd/server` puede coordinar contextos, pero no implementa dominio.
- `assistant` puede llamar al puerto MCP, nunca a PostgreSQL o TigerBeetle.
- `mcp` puede llamar al caso de uso `ledger`, nunca escribir balances.
- `ledger` es el único contexto autorizado para movimientos financieros.
- `seed` escribe identidades y staging validado; el replay financiero debe ser
  un caso de uso explícito del ledger.
- Web y mobile consumen el contrato OpenAPI y no duplican ownership, saldo ni
  reglas de confirmación.

## Pruebas

Las pruebas viven junto al contexto que protegen y usan el sufijo `_test.go`:

- `assistant/*_test.go`: interpretación de lenguaje y estados conversacionales.
- `ledger/*_test.go`: montos, filtros, invariantes e idempotencia.
- `mcp/*_test.go`: normalización y estados de aprobación.
- `auth/*_test.go`: credenciales, MFA y sesiones.
- `seed/*_test.go`: validación y reportes de fixtures.
- `cmd/server/*_test.go`: contrato HTTP y traducción de errores.

Los tests de integración que requieren Docker deben estar separados de los
unitarios y usar datos efímeros; no deben depender de cuentas personales ni de
fixtures de producción.
