param(
    [string]$ProtocolRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$scriptPath = Join-Path $ProtocolRoot 'scripts\gen-go.ps1'
$generatedPath = Join-Path $ProtocolRoot 'gen\go\gameagent\protocol\v1alpha2'
$pbGo = Join-Path $generatedPath 'gameagent.pb.go'
$grpcGo = Join-Path $generatedPath 'gameagent_grpc.pb.go'
$violations = New-Object System.Collections.Generic.List[string]

function Add-Violation {
    param([string]$Message)
    $violations.Add($Message) | Out-Null
}

if (-not (Test-Path -LiteralPath $scriptPath)) {
    Add-Violation "missing Go generation script: $scriptPath"
} else {
    & powershell -ExecutionPolicy Bypass -File $scriptPath -ProtocolRoot $ProtocolRoot
    if ($LASTEXITCODE -ne 0) {
        Add-Violation "Go generation script failed with exit code $LASTEXITCODE"
    }
}

foreach ($file in @($pbGo, $grpcGo)) {
    if (-not (Test-Path -LiteralPath $file)) {
        Add-Violation "missing generated Go file: $file"
        continue
    }

    $firstLines = Get-Content -LiteralPath $file -TotalCount 5 -Encoding UTF8
    if (-not ($firstLines -match 'Code generated|DO NOT EDIT')) {
        Add-Violation "generated Go file has no generated marker: $file"
    }
}

if (Test-Path -LiteralPath $pbGo) {
    $pbSource = Get-Content -LiteralPath $pbGo -Raw -Encoding UTF8
    if ($pbSource -notmatch 'package\s+protocolv1alpha2') {
        Add-Violation 'generated Go protobuf package must be protocolv1alpha2'
    }
}

if (Test-Path -LiteralPath $grpcGo) {
    $grpcSource = Get-Content -LiteralPath $grpcGo -Raw -Encoding UTF8
    if ($grpcSource -notmatch 'type\s+GameAgentGatewayClient\s+interface') {
        Add-Violation 'generated Go gRPC client interface is missing'
    }
    if ($grpcSource -notmatch 'type\s+GameAgentGatewayServer\s+interface') {
        Add-Violation 'generated Go gRPC server interface is missing'
    }
}

if ($violations.Count -gt 0) {
    Write-Host 'Go generation check failed:'
    foreach ($violation in $violations) {
        Write-Host " - $violation"
    }
    exit 1
}

Write-Host 'Go generation check passed.'
