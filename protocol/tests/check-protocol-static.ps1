param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$protoPath = Join-Path $Root 'proto\gameagent.proto'
$violations = New-Object System.Collections.Generic.List[string]

function Add-Violation {
    param([string]$Message)
    $violations.Add($Message) | Out-Null
}

if (-not (Test-Path -LiteralPath $protoPath)) {
    Add-Violation "missing protocol proto file: $protoPath"
} else {
    $proto = Get-Content -LiteralPath $protoPath -Raw -Encoding UTF8

    $requiredPatterns = @(
        'syntax\s*=\s*"proto3";',
        'package\s+gameagent\.protocol\.v1alpha1;',
        'import\s+"google/protobuf/struct\.proto";',
        'option\s+csharp_namespace\s*=\s*"GameAgent\.Protocol\.V1Alpha1";',
        'option\s+go_package\s*=\s*"gameagent/protocol/gen/go/gameagent/protocol/v1alpha1;protocolv1alpha1";',
        'service\s+GameAgentGateway\s*\{',
        'rpc\s+Connect\s*\(\s*stream\s+AdapterMessage\s*\)\s+returns\s*\(\s*stream\s+RuntimeMessage\s*\);',
        'message\s+AdapterHello\s*\{',
        'message\s+EnvironmentReady\s*\{',
        'message\s+EntityRef\s*\{',
        'message\s+GameTime\s*\{',
        'message\s+GameEvent\s*\{',
        'enum\s+EventAckStatus\s*\{',
        'EVENT_ACK_STATUS_ACCEPTED\s*=\s*1;',
        'message\s+EventAck\s*\{',
        'message\s+ObserveRequest\s*\{',
        'message\s+Observation\s*\{',
        'enum\s+ExecutionMode\s*\{',
        'message\s+Capability\s*\{',
        'message\s+CapabilityRequest\s*\{',
        'message\s+CapabilityList\s*\{',
        'message\s+ActionRequest\s*\{',
        'enum\s+ActionStatus\s*\{',
        'ACTION_STATUS_CANCELLED\s*=\s*7;',
        'ACTION_STATUS_REJECTED\s*=\s*8;',
        'message\s+ActionStatusUpdate\s*\{',
        'message\s+ActionResult\s*\{',
        'message\s+CancelActionRequest\s*\{',
        'message\s+Error\s*\{',
        'message\s+Heartbeat\s*\{',
        'oneof\s+payload\s*\{'
    )

    foreach ($pattern in $requiredPatterns) {
        if ($proto -notmatch $pattern) {
            Add-Violation "missing required proto pattern: $pattern"
        }
    }

    if ($proto -notmatch 'message\s+CapabilityRequest\s*\{[^}]*optional\s+string\s+entity_id\s*=\s*1;') {
        Add-Violation 'CapabilityRequest.entity_id must be optional'
    }

    if ($proto -notmatch 'message\s+CapabilityList\s*\{[^}]*optional\s+string\s+entity_id\s*=\s*1;') {
        Add-Violation 'CapabilityList.entity_id must be optional'
    }

    if ($proto -notmatch 'message\s+AdapterHello\s*\{[^}]*string\s+session_id\s*=\s*6;') {
        Add-Violation 'AdapterHello.session_id must use field 6'
    }

    if ($proto -notmatch 'message\s+EnvironmentReady\s*\{[^}]*string\s+session_id\s*=\s*1;') {
        Add-Violation 'EnvironmentReady.session_id must use field 1'
    }

    if ($proto -match 'ProtocolError') {
        Add-Violation 'proto must use Error, not ProtocolError'
    }

    if ($proto -match 'instance_id') {
        Add-Violation 'v1alpha1 must use session_id, not instance_id'
    }

    if ($proto -match 'agent_id') {
        Add-Violation 'v1alpha1 must not expose agent_id to adapters'
    }
}

if ($violations.Count -gt 0) {
    Write-Host 'Protocol static check failed:'
    foreach ($violation in $violations) {
        Write-Host " - $violation"
    }
    exit 1
}

Write-Host 'Protocol static check passed.'
