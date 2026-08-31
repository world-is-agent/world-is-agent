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
        'package\s+gameagent\.protocol\.v1alpha2;',
        'import\s+"google/protobuf/struct\.proto";',
        'option\s+csharp_namespace\s*=\s*"GameAgent\.Protocol\.V1Alpha2";',
        'option\s+go_package\s*=\s*"gameagent/protocol/gen/go/gameagent/protocol/v1alpha2;protocolv1alpha2";',
        'service\s+GameAgentGateway\s*\{',
        'rpc\s+Connect\s*\(\s*stream\s+AdapterMessage\s*\)\s+returns\s*\(\s*stream\s+RuntimeMessage\s*\);',
        'message\s+AdapterHello\s*\{',
        'message\s+EnvironmentReady\s*\{',
        'message\s+EntityRef\s*\{',
        'message\s+EntityRef\s*\{[^}]*string\s+definition_id\s*=\s*4;',
        'message\s+GameTime\s*\{',
        'message\s+ContextFact\s*\{',
        'message\s+ContextFact\s*\{[^}]*string\s+kind\s*=\s*1;',
        'message\s+ContextFact\s*\{[^}]*string\s+actor_entity_id\s*=\s*2;',
        'message\s+ContextFact\s*\{[^}]*string\s+target_entity_id\s*=\s*3;',
        'message\s+ContextFact\s*\{[^}]*string\s+scope_id\s*=\s*4;',
        'message\s+ContextFact\s*\{[^}]*string\s+text\s*=\s*5;',
        'message\s+ContextFact\s*\{[^}]*string\s+label\s*=\s*6;',
        'message\s+ContextFact\s*\{[^}]*google\.protobuf\.Struct\s+attributes\s*=\s*7;',
        'message\s+GameEvent\s*\{',
        'message\s+GameEvent\s*\{[^}]*repeated\s+ContextFact\s+context_facts\s*=\s*9;',
        'enum\s+EventAckStatus\s*\{',
        'EVENT_ACK_STATUS_ACCEPTED\s*=\s*1;',
        'message\s+EventAck\s*\{',
        'message\s+ObserveRequest\s*\{',
        'message\s+Observation\s*\{',
        'enum\s+ExecutionMode\s*\{',
        'enum\s+CapabilityConcurrencyMode\s*\{',
        'CAPABILITY_CONCURRENCY_MODE_SEQUENTIAL\s*=\s*1;',
        'CAPABILITY_CONCURRENCY_MODE_PARALLEL_SAFE\s*=\s*2;',
        'message\s+Capability\s*\{',
        'message\s+Capability\s*\{[^}]*CapabilityConcurrencyMode\s+concurrency_mode\s*=\s*7;',
        'message\s+CapabilityRequest\s*\{',
        'message\s+CapabilityList\s*\{',
        'message\s+ActionRequest\s*\{',
        'message\s+ActionRequest\s*\{[^}]*string\s+source_event_id\s*=\s*7;',
        'message\s+ActionRequest\s*\{[^}]*string\s+source_turn_id\s*=\s*8;',
        'enum\s+ActionStatus\s*\{',
        'ACTION_STATUS_CANCELLED\s*=\s*7;',
        'ACTION_STATUS_REJECTED\s*=\s*8;',
        'message\s+ActionStatusUpdate\s*\{',
        'message\s+ActionResult\s*\{',
        'message\s+CancelActionRequest\s*\{',
        'enum\s+TurnCompletionStatus\s*\{',
        'TURN_COMPLETION_STATUS_COMPLETED\s*=\s*1;',
        'TURN_COMPLETION_STATUS_FAILED\s*=\s*2;',
        'TURN_COMPLETION_STATUS_CANCELLED\s*=\s*3;',
        'message\s+TurnCompletion\s*\{',
        'message\s+TurnCompletion\s*\{[^}]*string\s+turn_id\s*=\s*1;',
        'message\s+TurnCompletion\s*\{[^}]*string\s+event_id\s*=\s*2;',
        'message\s+TurnCompletion\s*\{[^}]*string\s+world_id\s*=\s*3;',
        'message\s+TurnCompletion\s*\{[^}]*string\s+entity_id\s*=\s*4;',
        'message\s+TurnCompletion\s*\{[^}]*TurnCompletionStatus\s+status\s*=\s*5;',
        'message\s+TurnCompletion\s*\{[^}]*Error\s+error\s*=\s*6;',
        'message\s+Error\s*\{',
        'message\s+Heartbeat\s*\{',
        'message\s+AdapterMessage\s*\{',
        'message\s+RuntimeMessage\s*\{',
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

    if ($proto -match 'message\s+AdapterHello\s*\{[^}]*save_id') {
        Add-Violation 'AdapterHello must not include save_id in v1alpha2'
    }

    if ($proto -match 'message\s+AdapterHello\s*\{[^}]*world_id') {
        Add-Violation 'AdapterHello must not include world_id in v1alpha2'
    }

    if ($proto -notmatch 'message\s+EnvironmentReady\s*\{[^}]*string\s+session_id\s*=\s*1;') {
        Add-Violation 'EnvironmentReady.session_id must use field 1'
    }

    if ($proto -notmatch 'message\s+GameEvent\s*\{[^}]*string\s+world_id\s*=\s*7;') {
        Add-Violation 'GameEvent.world_id must use field 7'
    }

    if ($proto -notmatch 'message\s+GameEvent\s*\{[^}]*string\s+target_entity_id\s*=\s*8;') {
        Add-Violation 'GameEvent.target_entity_id must use field 8'
    }

    if ($proto -match 'message\s+ContextFact\s*\{[^}]*definition_id') {
        Add-Violation 'ContextFact must not include definition_id'
    }

    if ($proto -notmatch 'message\s+ObserveRequest\s*\{[^}]*string\s+world_id\s*=\s*2;') {
        Add-Violation 'ObserveRequest.world_id must use field 2'
    }

    if ($proto -notmatch 'message\s+Observation\s*\{[^}]*string\s+world_id\s*=\s*7;') {
        Add-Violation 'Observation.world_id must use field 7'
    }

    if ($proto -match 'message\s+Observation\s*\{[^}]*definition_id') {
        Add-Violation 'Observation must not include definition_id'
    }

    if ($proto -notmatch 'message\s+ActionRequest\s*\{[^}]*string\s+world_id\s*=\s*6;') {
        Add-Violation 'ActionRequest.world_id must use field 6'
    }

    if ($proto -notmatch 'message\s+ActionRequest\s*\{[^}]*string\s+source_event_id\s*=\s*7;') {
        Add-Violation 'ActionRequest.source_event_id must use field 7'
    }

    if ($proto -notmatch 'message\s+ActionRequest\s*\{[^}]*string\s+source_turn_id\s*=\s*8;') {
        Add-Violation 'ActionRequest.source_turn_id must use field 8'
    }

    if ($proto -notmatch 'message\s+TurnCompletion\s*\{[^}]*string\s+turn_id\s*=\s*1;[^}]*string\s+event_id\s*=\s*2;[^}]*string\s+world_id\s*=\s*3;[^}]*string\s+entity_id\s*=\s*4;[^}]*TurnCompletionStatus\s+status\s*=\s*5;[^}]*Error\s+error\s*=\s*6;') {
        Add-Violation 'TurnCompletion fields must match approved field numbers'
    }

    if ($proto -notmatch 'EVENT_ACK_STATUS_DUPLICATE\s*=\s*2;') {
        Add-Violation 'EventAckStatus must keep DUPLICATE at field value 2'
    }

    if ($proto -notmatch 'message\s+Heartbeat\s*\{[^}]*uint64\s+last_event_sequence\s*=\s*2;') {
        Add-Violation 'Heartbeat.last_event_sequence must use field 2'
    }

    if ($proto -notmatch 'message\s+AdapterMessage\s*\{[^}]*AdapterHello\s+hello\s*=\s*10;[^}]*GameEvent\s+event\s*=\s*11;[^}]*Observation\s+observation\s*=\s*12;[^}]*CapabilityList\s+capabilities\s*=\s*13;[^}]*ActionStatusUpdate\s+action_status\s*=\s*14;[^}]*ActionResult\s+action_result\s*=\s*15;[^}]*Heartbeat\s+heartbeat\s*=\s*16;[^}]*Error\s+error\s*=\s*17;') {
        Add-Violation 'AdapterMessage envelope oneof must match v1alpha2 contract'
    }

    if ($proto -notmatch 'message\s+RuntimeMessage\s*\{[^}]*EnvironmentReady\s+environment_ready\s*=\s*10;[^}]*ObserveRequest\s+observe\s*=\s*11;[^}]*CapabilityRequest\s+capability_request\s*=\s*12;[^}]*ActionRequest\s+action\s*=\s*13;[^}]*CancelActionRequest\s+cancel_action\s*=\s*14;[^}]*EventAck\s+event_ack\s*=\s*15;[^}]*Error\s+error\s*=\s*16;[^}]*TurnCompletion\s+turn_completion\s*=\s*17;') {
        Add-Violation 'RuntimeMessage envelope oneof must match v1alpha2 contract'
    }

    if ($proto -match 'ProtocolError') {
        Add-Violation 'proto must use Error, not ProtocolError'
    }

    if ($proto -match 'save_id') {
        Add-Violation 'v1alpha2 must use world_id, not save_id'
    }

    if ($proto -match 'instance_id') {
        Add-Violation 'v1alpha2 must use session_id, not instance_id'
    }

    if ($proto -match 'agent_id') {
        Add-Violation 'v1alpha2 must not expose agent_id to adapters'
    }

    if ($proto -match 'target_definition_id') {
        Add-Violation 'proto must not include target_definition_id'
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
