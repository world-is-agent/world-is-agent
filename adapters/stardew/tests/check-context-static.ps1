param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
)

$ErrorActionPreference = 'Stop'

$requiredFiles = @(
    'GameAgent.Stardew.csproj',
    'manifest.json',
    'src/Events/PlayerInteractProbe.cs',
    'src/Events/PlayerInteractTargetSelector.cs',
    'src/State/StardewObservation.cs',
    'src/State/StardewObservationFactory.cs',
    'src/State/ObservationBuilder.cs',
    'src/Capabilities/SpeakCapability.cs',
    'src/Capabilities/EmoteCapability.cs',
    'src/Capabilities/PresentDialogueCapability.cs',
    'src/Capabilities/FacePlayerCapability.cs',
    'src/Capabilities/FacePlayerDirection.cs',
    'src/Dialogue/ConversationStateStore.cs',
    'src/Dialogue/PresentDialogueInput.cs',
    'src/Dialogue/DialogueInteractionController.cs',
    'src/Dialogue/DialogueInteractionMenu.cs',
    'src/Runtime/CapabilityCatalog.cs',
    'src/Runtime/ProtocolMapper.Core.cs',
    'src/Runtime/RuntimeClient.cs',
    'src/Runtime/RuntimeWorldScope.cs',
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

function Reject-Content {
    param(
        [string]$File,
        [string]$Pattern,
        [string]$Message
    )

    $path = Join-Path $Root $File
    if ((Test-Path -LiteralPath $path) -and (Select-String -LiteralPath $path -Pattern $Pattern -CaseSensitive -Quiet)) {
        $failures.Add($Message) | Out-Null
    }
}

Require-Content 'src/State/StardewObservation.cs' 'sealed record StardewObservation' 'StardewObservation model must be the adapter current-fact schema.'
Require-Content 'src/State/StardewObservation.cs' 'StardewConversation' 'StardewObservation model must include conversation state.'
Require-Content 'src/State/StardewObservationFactory.cs' 'sealed class StardewObservationFactory' 'StardewObservationFactory must normalize Stardew facts.'
Require-Content 'src/State/StardewObservationFactory.cs' 'day_of_month % 7|dayOfMonth % 7|DayOfMonth % 7' 'Factory must preserve deterministic weekday fallback.'
Require-Content 'src/State/ObservationBuilder.cs' 'Game1\.Date\.DayOfWeek' 'ObservationBuilder must prefer Stardew native DayOfWeek.'
Require-Content 'src/State/ObservationBuilder.cs' 'friendshipData\.ContainsKey' 'ObservationBuilder must use friendshipData.ContainsKey for relationship known.'
Require-Content 'src/State/ObservationBuilder.cs' 'ConversationStateStore' 'ObservationBuilder must use adapter conversation state as an observation source.'
Require-Content 'src/Dialogue/ConversationStateStore.cs' 'IConversationIdGenerator' 'ConversationStateStore must accept an injected conversation id generator.'
Require-Content 'src/Dialogue/ConversationStateStore.cs' 'CommitPending' 'ConversationStateStore must commit pending mutation after EventAck.ACCEPTED.'
Require-Content 'src/Events/PlayerInteractTrigger.cs' 'action_button' 'Player interaction trigger must include action_button.'
Require-Content 'src/Events/PlayerInteractTrigger.cs' 'mouse_left' 'Player interaction trigger must include mouse_left.'
Require-Content 'src/Events/PlayerInteractTrigger.cs' 'mouse_right' 'Player interaction trigger must include mouse_right.'
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' '"stardew"' 'ProtocolMapper must write Observation.state.stardew.'
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' 'NearbyEntities' 'ProtocolMapper must publish nearby entity refs.'
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' 'player_said_to_npc' 'ProtocolMapper must build player_said_to_npc events.'
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' 'conversation_id' 'ProtocolMapper must carry conversation_id in dialogue events and observation.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'present_dialogue' 'CapabilityCatalog must register present_dialogue.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'face_player' 'CapabilityCatalog must register face_player.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'EventAckStatus\.Accepted' 'RuntimeClient must commit conversation state after accepted EventAck.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'TryConsumeCancelled\(request\.ActionId\)' 'RuntimeClient must let delayed dialogue display honor CancelAction.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'nearby_npcs_omitted_count' 'ProtocolMapper tests must cover nearby NPC truncation.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'friendship_points' 'ProtocolMapper tests must cover relationship visibility.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'present_dialogue' 'ProtocolMapper tests must cover present_dialogue.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'ConversationStateStore' 'ProtocolMapper tests must cover conversation state.'

Reject-Content 'src/State/StardewObservationFactory.cs' 'using StardewValley|StardewValley\.|Game1\.|\bNPC\b|\bFarmer\b' 'StardewObservationFactory must not reference Stardew live objects.'
Reject-Content 'src/Capabilities/PresentDialogueCapability.cs' 'GameAgent\.Stardew\.Runtime|ProtocolMapper' 'PresentDialogueCapability must not depend on Runtime mapper.'
Reject-Content 'src/Runtime/ProtocolMapper.Core.cs' '\["agent_id"\]|\["agent_tile_x"\]|\["player_tile_x"\]|\["friendship"\]' 'ProtocolMapper must not write legacy flat observation state.'
Reject-Content 'src/Runtime/RuntimeClient.cs' 'ProbeObservation' 'RuntimeClient must use StardewObservation in the production observation path.'
Reject-Content 'src/Runtime/ProtocolMapper.Core.cs' 'ProbeObservation' 'ProtocolMapper must not consume ProbeObservation.'
Reject-Content 'tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj' 'ProbeObservation' 'ProtocolMapper tests must not compile ProbeObservation.'

if ($failures.Count -gt 0) {
    Write-Host 'Stardew adapter context static check failed:'
    foreach ($failure in $failures) {
        Write-Host " - $failure"
    }
    exit 1
}

Write-Host 'Stardew adapter context static check passed.'
