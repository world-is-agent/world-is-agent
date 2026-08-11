param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$violations = New-Object System.Collections.Generic.List[string]

function Add-Violation {
    param([string]$Message)
    $violations.Add($Message) | Out-Null
}

function Search-Files {
    param(
        [string]$Path,
        [string[]]$Include,
        [string]$Pattern
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return @()
    }

    Get-ChildItem -LiteralPath $Path -Recurse -File -Include $Include |
        Select-String -Pattern $Pattern -CaseSensitive:$false
}

$runtimePath = Join-Path $Root 'runtime'
$adapterPath = Join-Path $Root 'adapters'
$stardewPath = Join-Path $adapterPath 'stardew'

$runtimeForbiddenTerms = @(
    'SMAPI',
    'Game1',
    'Farmer',
    'Abigail',
    'PelicanTown',
    'StardewValley',
    'Stardew',
    'Minecraft',
    'Unity',
    'Godot'
)

$sourceIncludes = @('*.go', '*.cs', '*.proto', '*.json', '*.yaml', '*.yml')

foreach ($term in $runtimeForbiddenTerms) {
    $matches = Search-Files -Path $runtimePath -Include $sourceIncludes -Pattern "\b$term\b"
    foreach ($match in $matches) {
        Add-Violation "runtime contains game-specific term '$term': $($match.Path):$($match.LineNumber)"
    }
}

$runtimeAdapterRefs = Search-Files -Path $runtimePath -Include $sourceIncludes -Pattern 'adapters[/\\]|[/\\]adapters[/\\]|adapters\.'
foreach ($match in $runtimeAdapterRefs) {
    Add-Violation "runtime references adapters: $($match.Path):$($match.LineNumber)"
}

$stardewRuntimeInternalRefs = Search-Files -Path $stardewPath -Include $sourceIncludes -Pattern 'runtime[/\\]internal|runtime\.internal'
foreach ($match in $stardewRuntimeInternalRefs) {
    Add-Violation "stardew adapter references runtime/internal: $($match.Path):$($match.LineNumber)"
}

$protocolGenPath = Join-Path $Root 'protocol\gen'
if (Test-Path -LiteralPath $protocolGenPath) {
    $generatedFiles = Get-ChildItem -LiteralPath $protocolGenPath -Recurse -File
    foreach ($file in $generatedFiles) {
        $firstLines = Get-Content -LiteralPath $file.FullName -TotalCount 5 -ErrorAction SilentlyContinue
        $hasGeneratedMarker = $firstLines -match 'generated|do not edit|Code generated'
        if (-not $hasGeneratedMarker) {
            Add-Violation "protocol generated file has no generated marker: $($file.FullName)"
        }
    }
}

$protocolStaticCheck = Join-Path $Root 'protocol\tests\check-protocol-static.ps1'
if (Test-Path -LiteralPath $protocolStaticCheck) {
    & powershell -ExecutionPolicy Bypass -File $protocolStaticCheck -Root (Join-Path $Root 'protocol')
    if ($LASTEXITCODE -ne 0) {
        Add-Violation "protocol static check failed"
    }
}

if ($violations.Count -gt 0) {
    Write-Host 'Architecture check failed:'
    foreach ($violation in $violations) {
        Write-Host " - $violation"
    }
    exit 1
}

Write-Host 'Architecture check passed.'
