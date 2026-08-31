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
    'src/Dialogue/DialogueReplyChoice.cs',
    'src/Dialogue/DialogueResponseMenuLayout.cs',
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
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' 'ContextFacts' 'ProtocolMapper must attach model-visible context facts to player dialogue events.'
Require-Content 'src/Runtime/ProtocolMapper.Core.cs' '"utterance"' 'ProtocolMapper must mark player dialogue context facts as utterance.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'present_dialogue' 'CapabilityCatalog must register present_dialogue.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'face_player' 'CapabilityCatalog must register face_player.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'tool_policy' 'CapabilityCatalog must publish GameAgent tool policy metadata.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'exclusive_per_step' 'present_dialogue must declare exclusive_per_step policy.'
Require-Content 'src/Runtime/CapabilityCatalog.cs' 'settle_after_success' 'present_dialogue must declare settle_after_success policy.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'EventAckStatus\.Accepted' 'RuntimeClient must commit conversation state after accepted EventAck.'
Require-Content 'src/Runtime/RuntimeClient.cs' 'TryConsumeCancelled\(request\.ActionId\)' 'RuntimeClient must let delayed dialogue display honor CancelAction.'
Require-Content 'src/Dialogue/DialogueInteractionController.cs' 'Game1\.DrawDialogue\(new StardewValley\.Dialogue' 'Dialogue UI must show the NPC line through Stardew native dialogue first.'
Require-Content 'src/Dialogue/DialogueInteractionController.cs' 'new DialogueInteractionMenu' 'Dialogue UI must show reply choices in the adapter bottom response menu after the native NPC dialogue advances.'
Require-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'IKeyboardSubscriber' 'Dialogue response menu must own a keyboard subscriber for inline free-text input.'
Require-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'keyboardDispatcher\.Subscriber' 'Dialogue response menu must route keyboard input to its inline free-text row.'
Require-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'BodyFont\s*=>\s*Game1\.dialogueFont' 'Dialogue response body text must use Stardew dialogueFont for NPC-dialogue visual consistency.'
Require-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'dialogue_option' 'Dialogue response menu must submit clicked generated reply options.'
Require-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'dialogue_free_text' 'Dialogue response menu must submit inline free-text replies.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'nearby_npcs_omitted_count' 'ProtocolMapper tests must cover nearby NPC truncation.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'friendship_points' 'ProtocolMapper tests must cover relationship visibility.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'present_dialogue' 'ProtocolMapper tests must cover present_dialogue.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'ConversationStateStore' 'ProtocolMapper tests must cover conversation state.'
Require-Content 'tests/ProtocolMapper.Tests/Program.cs' 'ContextFacts' 'ProtocolMapper tests must cover player dialogue context facts.'

Reject-Content 'src/State/StardewObservationFactory.cs' 'using StardewValley|StardewValley\.|Game1\.|\bNPC\b|\bFarmer\b' 'StardewObservationFactory must not reference Stardew live objects.'
Reject-Content 'src/Capabilities/PresentDialogueCapability.cs' 'GameAgent\.Stardew\.Runtime|ProtocolMapper' 'PresentDialogueCapability must not depend on Runtime mapper.'
Reject-Content 'src/Runtime/ProtocolMapper.Core.cs' '\["agent_id"\]|\["agent_tile_x"\]|\["player_tile_x"\]|\["friendship"\]' 'ProtocolMapper must not write legacy flat observation state.'
Reject-Content 'src/Runtime/RuntimeClient.cs' 'ProbeObservation' 'RuntimeClient must use StardewObservation in the production observation path.'
Reject-Content 'src/Runtime/ProtocolMapper.Core.cs' 'ProbeObservation' 'ProtocolMapper must not consume ProbeObservation.'
Reject-Content 'tests/ProtocolMapper.Tests/ProtocolMapper.Tests.csproj' 'ProbeObservation' 'ProtocolMapper tests must not compile ProbeObservation.'
Reject-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'DrawWrappedText\(b, this\.input\.Text' 'DialogueInteractionMenu must not draw the NPC line in the same custom menu as choices/input.'
Reject-Content 'src/Dialogue/DialogueInteractionController.cs' 'createQuestionDialogue' 'Dialogue UI must not use Stardew native question UI for replies because its rows cannot accept inline text input.'
Reject-Content 'src/Dialogue/DialogueReplyChoice.cs' 'Something else' 'Inline free text must not be exposed as a clickable Something else choice.'
Reject-Content 'src/Dialogue/DialogueInteractionMenu.cs' 'DrawWrappedText\(b, label, Game1\.smallFont|DrawWrappedText\(b, this\.Text, Game1\.smallFont|DrawWrappedText\(b, Placeholder, Game1\.smallFont|WrapText\(text, Game1\.smallFont|Game1\.smallFont\.MeasureString\(lastLine\)' 'Dialogue response and input body text must not be drawn with smallFont.'

$dialogueControllerPath = Join-Path $Root 'src/Dialogue/DialogueInteractionController.cs'
if (Test-Path -LiteralPath $dialogueControllerPath) {
    $dialogueControllerSource = Get-Content -LiteralPath $dialogueControllerPath -Raw
    if ($dialogueControllerSource -notmatch 'flow\.Start\(\(\) => Game1\.DrawDialogue\(new StardewValley\.Dialogue') {
        $failures.Add('DialogueInteractionController must start DialoguePresentationFlow with Stardew native DrawDialogue as the NPC-line display action.') | Out-Null
    }
}

$dialogueFlowPath = Join-Path $Root 'src/Dialogue/DialoguePresentationFlow.cs'
if (Test-Path -LiteralPath $dialogueFlowPath) {
    $dialogueFlowSource = Get-Content -LiteralPath $dialogueFlowPath -Raw
    if ($dialogueFlowSource -notmatch 'showNpcLine\(\);\s*\r?\n\s*this\.MarkDisplayed\(\);') {
        $failures.Add('DialoguePresentationFlow must mark displayed immediately after showing the NPC line.') | Out-Null
    }

    if ($dialogueFlowSource -match 'shouldShowReplyMenu[\s\S]{0,120}MarkDisplayed') {
        $failures.Add('present_dialogue ActionResult must not be gated on reply menu availability; sync action timeout is shorter than player reading time.') | Out-Null
    }
}

if ($failures.Count -gt 0) {
    Write-Host 'Stardew adapter context static check failed:'
    foreach ($failure in $failures) {
        Write-Host " - $failure"
    }
    exit 1
}

Write-Host 'Stardew adapter context static check passed.'
