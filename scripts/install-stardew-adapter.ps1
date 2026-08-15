param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
    [string]$GamePath = (Resolve-Path (Join-Path $PSScriptRoot '..\..\Stardew Valley')).Path,
    [string]$Configuration = 'Debug'
)

$ErrorActionPreference = 'Stop'

$projectPath = Join-Path $Root 'adapters\stardew\GameAgent.Stardew.csproj'
$outputPath = Join-Path $Root "adapters\stardew\bin\$Configuration"
$modsPath = Join-Path $GamePath 'Mods'
$targetPath = Join-Path $modsPath 'GameAgentStardew'

if (-not (Test-Path -LiteralPath $projectPath)) {
    throw "Stardew adapter project not found: $projectPath"
}

if (-not (Test-Path -LiteralPath $modsPath)) {
    throw "Stardew Mods directory not found: $modsPath"
}

Write-Host "Building Stardew adapter ($Configuration)..."
dotnet build $projectPath --configuration $Configuration
if ($LASTEXITCODE -ne 0) {
    throw "dotnet build failed with exit code $LASTEXITCODE"
}

if (-not (Test-Path -LiteralPath $outputPath)) {
    throw "Build output not found: $outputPath"
}

New-Item -ItemType Directory -Force -Path $targetPath | Out-Null

Write-Host "Installing adapter to: $targetPath"
Copy-Item -Path (Join-Path $outputPath '*') -Destination $targetPath -Recurse -Force

Write-Host 'Stardew adapter installed.'
