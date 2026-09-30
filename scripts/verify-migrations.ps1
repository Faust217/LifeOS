$ErrorActionPreference = "Stop"

function Invoke-Migration {
    param([string[]]$Arguments)

    $output = & go run ./cmd/lifeos-migrate @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Migration command failed: $($Arguments -join ' ')"
    }
    return ($output | Out-String).Trim()
}

$migrationDirectory = Join-Path $PSScriptRoot "..\db\migrations"
$upMigrations = @(Get-ChildItem -LiteralPath $migrationDirectory -File -Filter "*.up.sql" | Sort-Object Name)
if ($upMigrations.Count -eq 0) {
    throw "No production up migrations were found."
}

$versions = @{}
foreach ($migration in $upMigrations) {
    if ($migration.Name -notmatch '^(\d{6})_[a-z0-9_]+\.up\.sql$') {
        throw "Invalid migration filename: $($migration.Name)"
    }
    $version = $Matches[1]
    if ($versions.ContainsKey($version)) {
        throw "Duplicate migration version: $version"
    }
    $versions[$version] = $true

    $downName = $migration.Name -replace '\.up\.sql$', '.down.sql'
    if (-not (Test-Path -LiteralPath (Join-Path $migrationDirectory $downName) -PathType Leaf)) {
        throw "Missing paired down migration: $downName"
    }
}

$expectedVersion = [uint32]($upMigrations[-1].Name.Substring(0, 6))

$upOutput = Invoke-Migration @("up")
Write-Output $upOutput

$versionOutput = Invoke-Migration @("version")
Write-Output $versionOutput
if ($versionOutput -notmatch "migration version: $expectedVersion dirty=false") {
    throw "Production migration version did not match expected clean version $expectedVersion."
}

$repeatOutput = Invoke-Migration @("up")
Write-Output $repeatOutput
if ($repeatOutput -notmatch "no change") {
    throw "Repeated production migration up did not report no change."
}

$fixturePath = "internal/platform/migrations/testdata/smoke"
$fixtureTable = "foundation_migration_smoke"
Write-Output (Invoke-Migration @("-path", $fixturePath, "-table", $fixtureTable, "up"))

$fixtureVersion = Invoke-Migration @("-path", $fixturePath, "-table", $fixtureTable, "version")
Write-Output $fixtureVersion
if ($fixtureVersion -notmatch "migration version: 1 dirty=false") {
    throw "Smoke fixture did not reach clean version 1."
}

Write-Output (Invoke-Migration @("-path", $fixturePath, "-table", $fixtureTable, "down"))

$fixtureRollback = Invoke-Migration @("-path", $fixturePath, "-table", $fixtureTable, "version")
Write-Output $fixtureRollback
if ($fixtureRollback -notmatch "migration version: nil dirty=false") {
    throw "Smoke fixture did not return to a clean nil version."
}

Write-Output "Migration chain verification passed."
