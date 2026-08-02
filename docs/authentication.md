# Autenticación e identidad

## Principios

- PostgreSQL almacena usuarios, hashes de contraseña, sesiones y auditoría.
- Las contraseñas se procesan con bcrypt y nunca se almacenan en texto plano.
- Los access y refresh tokens son opacos, aleatorios y solo se almacenan como
  hashes SHA-256.
- Refresh rota ambos tokens y deja inutilizable el refresh token anterior.
- Logout revoca la sesión y es idempotente.
- La web conserva la sesión solo en `sessionStorage`, la restaura al recargar
  y rota el refresh token antes de que expire el access token; cerrar la
  pestaña elimina ese estado del navegador.
- Las rutas financieras requieren MFA habilitado y una marca
  `mfa_verified_at` en la sesión concreta; una sesión antigua no puede
  reutilizar el MFA de otra sesión.
- Los mensajes HTTP no revelan si un email existe ni detalles internos de SQL.
- El registro exige una contraseña de 8 a 72 bytes; se recomienda combinar
  letras, números y símbolos. El límite superior mantiene compatibilidad con
  bcrypt.
- Una cuenta con MFA activo no puede reemplazar su secreto mediante el endpoint
  de enrolamiento. La recuperación o rotación debe pasar por un flujo explícito
  de soporte/recuperación, no por un token de sesión ordinario.

## Contratos

El contrato machine-readable de esta superficie está en
[`docs/openapi.yaml`](openapi.yaml).

`POST /api/v1/auth/register` recibe `email`, `password` y `full_name`. Además
de la identidad, provisiona la cuenta HNL de checking inicial.

`POST /api/v1/auth/login` recibe `email` y `password` y devuelve el usuario,
`access_token`, `refresh_token` y sus expiraciones.

`POST /api/v1/auth/refresh` recibe `refresh_token`. Cada uso exitoso rota los
dos tokens.

`POST /api/v1/auth/logout` requiere el access token en el header Bearer y
revoca la sesión asociada.

## OAuth acotado

Google y GitHub se habilitan únicamente cuando el proveedor tiene configurados
su client ID, client secret y callback URL. El flujo es:

1. `GET /api/v1/auth/oauth/{provider}/start?return_to=...` genera un `state` aleatorio,
   almacenado solo como hash, lo vincula a una cookie `HttpOnly`/`SameSite` y
   redirige al proveedor. `return_to` debe estar incluido literalmente en
   `OAUTH_ALLOWED_REDIRECT_URIS`; así el callback no se convierte en un
   open redirect. La web usa su origen y la app usa `hypernova://oauth`.
2. `GET /api/v1/auth/oauth/{provider}/callback` consume el `state`, valida el
   perfil externo y redirige al `return_to` con un `oauth_code` efímero; sin
   `return_to`, devuelve ese código como JSON. Nunca devuelve tokens del
   proveedor ni tokens de sesión.
3. `POST /api/v1/auth/oauth/{provider}/exchange` consume el código una sola vez
   al crear la sesión. Si la cuenta tiene MFA, requiere `mfa_code`.

La vinculación usa el identificador estable del proveedor (`sub` de Google o
`id` de GitHub), no el email como única identidad. Si un email ya pertenece a
una cuenta local sin esa vinculación, el backend devuelve
`oauth_email_conflict`; la vinculación autenticada se mantiene fuera de este
slice para no convertir un email en una prueba suficiente de control de cuenta.

## Migraciones y seed

Las migraciones se aplican al arrancar la API y se registran en
`schema_migrations`. El comando `scripts/seed-users.ps1` carga únicamente la
colección `users` de `datos-prueba-HNL.json`; las cuentas y transacciones
financieras se gestionan mediante TigerBeetle.
