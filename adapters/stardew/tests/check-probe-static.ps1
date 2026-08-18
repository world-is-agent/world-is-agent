param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$requiredFiles = @(
    'GameAgent.Stardew.csproj',
    'manifest.json',
    'src/ModEntry.cs',
    'src/Events/PlayerInteractProbe.cs',
    'src/State/ProbeObservation.cs',
    'src/State/ObservationBuilder.cs',
    'src/Capabilities/SpeakCapability.cs',
    'src/Capabilities/EmoteCapability.cs',
    'src/Runtime/ActionCancellationRegistry.cs'
)

$failures = New-Object System.Collections.Generic.List[string]

foreach ($file in $requiredFiles) {
    $path = Join-Path $Root $file
    if (-not (Test-Path -LiteralPath $path)) {
        $failures.Add("missing file: $file") | Out-Null
    }
}

function Require-Content {
    param(
        [string]$File,
        [string]$Pattern,
        [string]$Message
    )

    $path = Join-Path $Root $File
    if ((Test-Path -LiteralPath $path) -and -not (Select-String -LiteralPath $path -Pattern $Pattern -Quiet)) {
        $failures.Add($Message) | Out-Null
    }
}

Require-Content 'src/ModEntry.cs' 'helper\.Events\.Input\.ButtonPressed' 'ModEntry must register ButtonPressed.'
Require-Content 'src/Events/PlayerInteractProbe.cs' 'IsActionButton\(\)' 'Probe must listen to Stardew action-button input.'
Require-Content 'src/Events/PlayerInteractProbe.cs' 'MouseLeft|MouseRight' 'Probe must treat mouse clicks as candidate NPC interaction input.'
Require-Content 'src/Events/PlayerInteractProbe.cs' 'Suppress\(e\.Button\)' 'Probe must suppress handled input so vanilla dialogue does not override probe dialogue.'
Require-Content 'src/ModEntry.cs' 'Linus' 'Probe must target Linus for this spike.'
Require-Content 'manifest.json' 'Linus' 'Manifest should describe the Linus probe target.'
Require-Content 'src/State/ObservationBuilder.cs' 'getFriendshipLevelForNPC' 'Observation must read player friendship with the NPC.'
Require-Content 'src/Capabilities/SpeakCapability.cs' 'DrawDialogue' 'Speak capability must use Stardew dialogue display.'
Require-Content 'src/Capabilities/SpeakCapability.cs' 'Hello from GameAgent' 'Spike must keep the hardcoded probe text.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'Name = "emote"' 'CapabilityCatalog must advertise the emote capability.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'happy.*sad.*surprised.*neutral' 'Emote capability schema must expose the supported emote names.'
Require-Content 'src/Runtime/RuntimeClient.cs' '"emote" => this\.HandleEmoteAction' 'RuntimeClient must route emote ActionRequest messages.'
Require-Content 'src/Runtime/ProtocolMapper.cs' 'RequireEmoteArgument' 'ProtocolMapper must parse emote ActionRequest arguments.'
Require-Content 'src/Capabilities/EmoteCapability.cs' 'doEmote' 'Emote capability must use Stardew NPC.doEmote.'
Require-Content 'src/Runtime/ActionCancellationRegistry.cs' 'ConcurrentDictionary' 'ActionCancellationRegistry must use a concurrent collection.'
Require-Content 'src/Runtime/ActionCancellationRegistry.cs' 'MarkCancelled' 'ActionCancellationRegistry must expose MarkCancelled.'
Require-Content 'src/Runtime/ActionCancellationRegistry.cs' 'TryConsumeCancelled' 'ActionCancellationRegistry must expose TryConsumeCancelled.'
Require-Content 'src/Runtime/ActionCancellationRegistry.cs' 'TryRemove' 'ActionCancellationRegistry must remove consumed cancel markers.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'case RuntimeMessage\.PayloadOneofCase\.CancelAction' 'RuntimeClient must handle CancelAction messages.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'HandleCancelAction\(message\.CancelAction\)' 'RuntimeClient must handle cancellation on the gRPC receive thread.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'MarkCancelled\(request\.ActionId\)' 'RuntimeClient must record cancelled action IDs.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'TryConsumeCancelled\(request\.ActionId' 'RuntimeClient must check cancellation before executing actions.'
Require-Content 'src/Runtime/ProtocolMapper.cs' 'BuildCancelledActionResult' 'ProtocolMapper must build cancelled ActionResult messages.'
Require-Content 'src/Runtime/ProtocolMapper.cs' 'ActionStatus\.Cancelled' 'Cancelled ActionResult must use ACTION_STATUS_CANCELLED.'

if ($failures.Count -gt 0) {
    Write-Host 'Stardew adapter probe static check failed:'
    foreach ($failure in $failures) {
        Write-Host " - $failure"
    }
    exit 1
}

Write-Host 'Stardew adapter probe static check passed.'
