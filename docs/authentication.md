# Autenticación de fase 1

## Principios

- PostgreSQL almacena usuarios, hashes de contraseña, sesiones y auditoría.
- Las contraseñas se procesan con bcrypt y nunca se almacenan en texto plano.
- Los access y refresh tokens son opacos, aleatorios y solo se almacenan como
  hashes SHA-256.
- Refresh rota ambos tokens y deja inutilizable el refresh token anterior.
- Logout revoca la sesión y es idempotente.
- Los mensajes HTTP no revelan si un email existe ni detalles internos de SQL.

## Contratos

El contrato machine-readable de esta superficie está en
[`docs/openapi.yaml`](openapi.yaml).

`POST /api/v1/auth/register` recibe `email`, `password` y `full_name`.

`POST /api/v1/auth/login` recibe `email` y `password` y devuelve el usuario,
`access_token`, `refresh_token` y sus expiraciones.

`POST /api/v1/auth/refresh` recibe `refresh_token`. Cada uso exitoso rota los
dos tokens.

`POST /api/v1/auth/logout` requiere el access token en el header Bearer y
revoca la sesión asociada.

## Migraciones y seed

Las migraciones se aplican al arrancar la API y se registran en
`schema_migrations`. El comando `scripts/seed-users.ps1` carga únicamente la
colección `users` de `datos-prueba-HNL.json`; `accounts` y `transactions` se
reservan para la fase 2.
