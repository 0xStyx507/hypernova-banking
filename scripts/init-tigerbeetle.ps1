$ErrorActionPreference = "Stop"

# This helper is intentionally idempotent: the TigerBeetle entrypoint formats
# the volume only when its data file does not exist.
Write-Host "TigerBeetle se inicializa automáticamente al arrancar el servicio." -ForegroundColor Cyan
Write-Host "El volumen persistente no se sobrescribe: docker compose up --build" -ForegroundColor Cyan

docker compose up --build -d postgres tigerbeetle
if ($LASTEXITCODE -ne 0) {
  throw "No se pudo iniciar PostgreSQL/TigerBeetle"
}

docker compose ps postgres tigerbeetle
