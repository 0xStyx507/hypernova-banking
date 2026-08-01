$ErrorActionPreference = "Stop"

# The fixture is mounted read-only in the API container; the seed command
# hashes passwords and imports only identity data during phase 1.
docker compose exec -T api /app/seed --file /seed/datos-prueba-HNL.json
