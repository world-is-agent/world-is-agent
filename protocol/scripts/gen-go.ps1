param(
    [string]$ProtocolRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path,
    [string]$ProtocInclude = $env:PROTOC_INCLUDE
)

$ErrorActionPreference = 'Stop'

$protocolRootPath = (Resolve-Path -LiteralPath $ProtocolRoot).Path
$repoRoot = (Resolve-Path (Join-Path $protocolRootPath '..')).Path
$protoRoot = Join-Path $protocolRootPath 'proto'
$protoFile = Join-Path $protoRoot 'gameagent.proto'

if (-not (Test-Path -LiteralPath $protoFile)) {
    throw "Missing proto file: $protoFile"
}

$protoc = Get-Command protoc -ErrorAction Stop
Get-Command protoc-gen-go -ErrorAction Stop | Out-Null
Get-Command protoc-gen-go-grpc -ErrorAction Stop | Out-Null

if ([string]::IsNullOrWhiteSpace($ProtocInclude)) {
    $protocRoot = Split-Path -Parent (Split-Path -Parent $protoc.Source)
    $candidateInclude = Join-Path $protocRoot 'include'
    if (Test-Path -LiteralPath (Join-Path $candidateInclude 'google\protobuf\struct.proto')) {
        $ProtocInclude = $candidateInclude
    }
}

if ([string]::IsNullOrWhiteSpace($ProtocInclude) -or -not (Test-Path -LiteralPath (Join-Path $ProtocInclude 'google\protobuf\struct.proto'))) {
    throw 'Unable to locate google/protobuf/struct.proto. Set PROTOC_INCLUDE or pass -ProtocInclude.'
}

& $protoc.Source `
    "--proto_path=$protoRoot" `
    "--proto_path=$ProtocInclude" `
    "--go_out=$repoRoot" `
    "--go_opt=module=gameagent" `
    "--go-grpc_out=$repoRoot" `
    "--go-grpc_opt=module=gameagent" `
    $protoFile

if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$generatedPath = Join-Path $protocolRootPath 'gen\go\gameagent\protocol\v1alpha1'
Write-Host "Generated Go protocol files under: $generatedPath"
