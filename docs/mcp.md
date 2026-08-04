# Contrato MCP v2

El MCP de Hypernova Banking expone `hypernova-mcp-http/2`. La versión cambia
cuando cambia un nombre de herramienta, su esquema de argumentos o la frontera
de seguridad `prepare -> confirm -> cancel`.

## Herramientas de lectura

Todas requieren una sesión autenticada y un `account_id` propiedad del usuario.
Los importes son cadenas de unidades menores USD.

| Herramienta | Propósito |
| --- | --- |
| `get_accounts` | Lista las cuentas activas del usuario. |
| `get_balance` | Consulta el saldo respaldado por TigerBeetle. |
| `get_transactions` | Devuelve movimientos recientes con paginación. |
| `search_transactions` | Filtra movimientos por fecha, tipo, dirección y monto. |
| `get_cashflow_summary` | Resume créditos, débitos, neto y cantidad de movimientos. |

`search_transactions` acepta `type`, `direction`, `from`, `to`, `min_amount`,
`max_amount` y `limit`. Las fechas usan RFC3339 y los filtros de monto usan
unidades menores.

## Herramientas financieras

`prepare_financial_action` solo persiste la intención. Nunca cambia el saldo.
La operación requiere una llamada separada a `confirm` con el PIN vigente, o
`cancel` para liberar la acción. El servidor valida ownership, moneda, monto,
idempotencia y estado antes de tocar TigerBeetle.

El asistente puede entender montos naturales como `USD 25.50`, `25,50 dólares`,
`cien dólares` y `dos mil`. Un entero sin símbolo conserva compatibilidad con el
contrato anterior y se interpreta como unidad menor (`2500` = `USD 25.00`).

## Reglas para clientes MCP

- No asumir que preparar equivale a ejecutar.
- Mostrar cuenta origen, cuenta destino, monto, moneda, tipo y expiración antes
  de confirmar.
- No enviar PIN, tokens ni credenciales al modelo.
- Reintentar confirmaciones con el mismo `action_id`; el ledger conserva la
  idempotencia.
- Tratar `unknown` y `confirming` como estados que requieren reconciliación,
  no como éxito automático.

## DDD y límites

`api/internal/mcp` administra intención y autorización de la acción. El
contexto `ledger` administra ownership, consultas y TigerBeetle. El contexto
`assistant` solo interpreta lenguaje y selecciona una herramienta permitida.
Los handlers HTTP traducen contratos y no contienen reglas financieras.
