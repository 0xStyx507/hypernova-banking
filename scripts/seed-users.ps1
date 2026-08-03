param(
  [string]$Fixture = "$(Join-Path (Get-Location) 'datos-prueba.json')"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $Fixture -PathType Leaf)) {
  throw "Fixture not found: $Fixture"
}

# Fixtures stay outside the image and are copied only for this explicit local
# operation. The seed command hashes passwords and imports identity data.
$containerFixture = "/tmp/$(Split-Path -Leaf $Fixture)"
docker compose cp --quiet $Fixture "api:$containerFixture"
try {
  docker compose exec -T api /app/seed --file $containerFixture
}
finally {
  docker compose exec -T api sh -c "rm -f '$containerFixture'" | Out-Null
}
