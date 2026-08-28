using System;
using System.Collections.Generic;
using System.Linq;
using Microsoft.Xna.Framework;
using Microsoft.Xna.Framework.Graphics;
using Microsoft.Xna.Framework.Input;
using StardewValley;
using StardewValley.Menus;

namespace GameAgent.Stardew.Dialogue;

public sealed record PlayerDialogueSubmission(
    string ConversationId,
    string InputKind,
    string Text,
    int? SelectedOptionIndex,
    string Trigger
);

public sealed class DialogueInteractionMenu : IClickableMenu
{
    private readonly string title;
    private readonly IReadOnlyList<DialogueReplyChoice> optionChoices;
    private readonly Action<PlayerDialogueSubmission> onSubmitted;
    private readonly Action onAbandoned;
    private readonly DialogueResponseMenuLayout layout;
    private readonly DialogueFreeTextInput? textInput;
    private bool submitted;
    private bool closedWithoutSubmission;
    private bool abandonmentNotified;

    private static SpriteFont BodyFont => Game1.dialogueFont;

    public DialogueInteractionMenu(
        string npcEntityId,
        string conversationId,
        string title,
        IReadOnlyList<DialogueReplyChoice> optionChoices,
        bool allowFreeText,
        Action<PlayerDialogueSubmission> onSubmitted,
        Action onAbandoned
    )
    {
        this.NpcEntityId = npcEntityId;
        this.ConversationId = conversationId;
        this.title = string.IsNullOrWhiteSpace(title) ? "Respond:" : title;
        this.optionChoices = (optionChoices ?? Array.Empty<DialogueReplyChoice>()).ToArray();
        this.onSubmitted = onSubmitted;
        this.onAbandoned = onAbandoned;
        this.layout = DialogueResponseMenuLayout.Build(
            Game1.uiViewport.Width,
            Game1.uiViewport.Height,
            this.optionChoices.Count,
            allowFreeText
        );

        this.xPositionOnScreen = this.layout.MenuBounds.X;
        this.yPositionOnScreen = this.layout.MenuBounds.Y;
        this.width = this.layout.MenuBounds.Width;
        this.height = this.layout.MenuBounds.Height;

        if (allowFreeText && this.layout.TextInputRow is DialogueMenuRectangle textInputRow && this.layout.TextInputTextArea is DialogueMenuRectangle textInputTextArea)
        {
            this.textInput = new DialogueFreeTextInput(ConversationStateStore.MaxLineTextChars)
            {
                Bounds = ToXna(textInputRow),
                TextArea = ToXna(textInputTextArea),
            };
            Game1.keyboardDispatcher.Subscriber = this.textInput;
        }
    }

    public string NpcEntityId { get; }
    public string ConversationId { get; }

    public void CloseWithoutSubmission()
    {
        this.closedWithoutSubmission = true;
        this.exitThisMenu();
    }

    public override void receiveLeftClick(int x, int y, bool playSound = true)
    {
        for (int i = 0; i < this.layout.OptionRows.Count; i++)
        {
            if (this.layout.OptionRows[i].Contains(x, y))
            {
                this.SubmitOption(this.optionChoices[i]);
                return;
            }
        }

        if (this.textInput is not null && this.layout.SendButton?.Contains(x, y) == true)
        {
            this.TrySubmitFreeText();
            return;
        }

        if (this.layout.CloseButton.Contains(x, y))
        {
            this.exitThisMenu();
            return;
        }

        if (this.textInput?.Bounds.Contains(x, y) == true)
            Game1.keyboardDispatcher.Subscriber = this.textInput;
    }

    public override void receiveKeyPress(Keys key)
    {
        if (key == Keys.Escape)
        {
            this.exitThisMenu();
            return;
        }

        if (key == Keys.Enter)
        {
            this.TrySubmitFreeText();
            return;
        }

        this.textInput?.RecieveSpecialInput(key);
    }

    public override void draw(SpriteBatch b)
    {
        IClickableMenu.drawTextureBox(b, this.xPositionOnScreen, this.yPositionOnScreen, this.width, this.height, Color.White);
        DrawLeftText(b, this.title, BodyFont, ToXna(this.layout.TitleArea), Game1.textColor);

        for (int i = 0; i < this.layout.OptionRows.Count; i++)
            DrawOptionRow(b, ToXna(this.layout.OptionRows[i]), this.optionChoices[i].Text);

        this.textInput?.Draw(b);

        DrawButton(b, ToXna(this.layout.CloseButton), "Close", enabled: true);
        if (this.textInput is not null && this.layout.SendButton is DialogueMenuRectangle sendButton)
            DrawButton(b, ToXna(sendButton), "Send", enabled: this.textInput.HasText);
        drawMouse(b);
    }

    protected override void cleanupBeforeExit()
    {
        if (this.textInput is not null && Game1.keyboardDispatcher.Subscriber == this.textInput)
            Game1.keyboardDispatcher.Subscriber = null;

        if (!this.submitted && !this.closedWithoutSubmission)
            this.NotifyAbandoned();

        base.cleanupBeforeExit();
    }

    private void SubmitOption(DialogueReplyChoice choice)
    {
        this.submitted = true;
        this.onSubmitted(new PlayerDialogueSubmission(
            this.ConversationId,
            "option",
            choice.Text,
            choice.SelectedOptionIndex,
            "dialogue_option"
        ));
        this.exitThisMenu();
    }

    private void TrySubmitFreeText()
    {
        string text = this.textInput?.Text.Trim() ?? string.Empty;
        if (text.Length == 0)
            return;

        this.submitted = true;
        this.onSubmitted(new PlayerDialogueSubmission(this.ConversationId, "free_text", text, null, "dialogue_free_text"));
        this.exitThisMenu();
    }

    private void NotifyAbandoned()
    {
        if (this.abandonmentNotified)
            return;

        this.abandonmentNotified = true;
        this.onAbandoned();
    }

    private static Rectangle ToXna(DialogueMenuRectangle rectangle)
    {
        return new Rectangle(rectangle.X, rectangle.Y, rectangle.Width, rectangle.Height);
    }

    private static void DrawButton(SpriteBatch b, Rectangle bounds, string label, bool enabled)
    {
        Color boxColor = enabled ? Color.White : Color.White * 0.55f;
        Color textColor = enabled ? Game1.textColor : Color.Gray;
        IClickableMenu.drawTextureBox(b, bounds.X, bounds.Y, bounds.Width, bounds.Height, boxColor);
        Vector2 size = BodyFont.MeasureString(label);
        Vector2 position = new(bounds.X + (bounds.Width - size.X) / 2, bounds.Y + (bounds.Height - size.Y) / 2);
        b.DrawString(BodyFont, label, position, textColor);
    }

    private static void DrawOptionRow(SpriteBatch b, Rectangle bounds, string label)
    {
        b.Draw(Game1.staminaRect, bounds, Color.Black * 0.08f);
        Rectangle textArea = new(bounds.X + 18, bounds.Y, bounds.Width - 36, bounds.Height);
        DrawFirstWrappedLine(b, label, BodyFont, Game1.textColor, textArea);
    }

    private static void DrawLeftText(SpriteBatch b, string text, SpriteFont font, Rectangle area, Color color)
    {
        Vector2 size = font.MeasureString(text);
        Vector2 position = new(area.X, area.Y + (area.Height - size.Y) / 2);
        b.DrawString(font, text, position, color);
    }

    private static void DrawFirstWrappedLine(SpriteBatch b, string text, SpriteFont font, Color color, Rectangle area)
    {
        int lineHeight = (int)font.MeasureString("A").Y;
        int y = GetCenteredTextY(area, lineHeight);
        string line = WrapText(text, font, area.Width).FirstOrDefault() ?? string.Empty;
        b.DrawString(font, line, new Vector2(area.X, y), color);
    }

    private static IReadOnlyList<string> WrapText(string text, SpriteFont font, int maxWidth)
    {
        var lines = new List<string>();
        string current = string.Empty;
        foreach (char c in text ?? string.Empty)
        {
            if (c == '\r')
                continue;

            if (c == '\n')
            {
                AddLine(lines, ref current);
                continue;
            }

            string candidate = current + c;
            if (current.Length > 0 && font.MeasureString(candidate).X > maxWidth)
            {
                AddLine(lines, ref current);
                current = c.ToString();
                continue;
            }

            current = candidate;
        }

        AddLine(lines, ref current);
        return lines;
    }

    private static void AddLine(List<string> lines, ref string current)
    {
        if (current.Length == 0)
            return;

        lines.Add(current);
        current = string.Empty;
    }

    private sealed class DialogueFreeTextInput : IKeyboardSubscriber
    {
        private const string Placeholder = "Type your response...";
        private readonly int characterLimit;

        public DialogueFreeTextInput(int characterLimit)
        {
            this.characterLimit = characterLimit;
        }

        public Rectangle Bounds { get; init; }
        public Rectangle TextArea { get; init; }
        public string Text { get; private set; } = string.Empty;
        public bool Selected { get; set; } = true;
        public bool HasText => !string.IsNullOrWhiteSpace(this.Text);

        public void Draw(SpriteBatch b)
        {
            IClickableMenu.drawTextureBox(b, this.Bounds.X, this.Bounds.Y, this.Bounds.Width, this.Bounds.Height, Color.White);
            Rectangle textArea = this.TextArea;

            if (this.Text.Length == 0)
            {
                DrawInputLine(b, Placeholder, Color.Gray, textArea);
                DrawCaret(b, textArea, 0);
                return;
            }

            DialogueSingleLineText line = DialogueSingleLineText.FitTrailingText(
                this.Text,
                textArea.Width,
                text => BodyFont.MeasureString(text).X
            );
            DrawInputLine(b, line.VisibleText, Game1.textColor, textArea);
            DrawCaret(b, textArea, line.CaretOffset);
        }

        public void RecieveTextInput(char inputChar)
        {
            if (inputChar == '\b')
            {
                this.RemoveLastCharacter();
                return;
            }

            if (inputChar == '\r' || inputChar == '\n' || char.IsControl(inputChar) || this.Text.Length >= this.characterLimit)
                return;

            this.Text += inputChar;
        }

        public void RecieveTextInput(string text)
        {
            foreach (char c in text ?? string.Empty)
                this.RecieveTextInput(c);
        }

        public void RecieveCommandInput(char command)
        {
            if (command == '\b')
                this.RemoveLastCharacter();
        }

        public void RecieveSpecialInput(Keys key)
        {
            if (key == Keys.Back)
                this.RemoveLastCharacter();
        }

        private void RemoveLastCharacter()
        {
            if (this.Text.Length > 0)
                this.Text = this.Text[..^1];
        }

        private static void DrawInputLine(SpriteBatch b, string text, Color color, Rectangle textArea)
        {
            int lineHeight = (int)BodyFont.MeasureString("A").Y;
            int y = GetCenteredTextY(textArea, lineHeight);
            b.DrawString(BodyFont, text, new Vector2(textArea.X, y), color);
        }

        private static void DrawCaret(SpriteBatch b, Rectangle textArea, float caretOffset)
        {
            int lineHeight = (int)BodyFont.MeasureString("A").Y;
            int caretX = Math.Min(textArea.Right - 2, textArea.X + (int)Math.Ceiling(caretOffset) + 2);
            int caretY = GetCenteredTextY(textArea, lineHeight);
            b.Draw(Game1.staminaRect, new Rectangle(caretX, caretY, 2, lineHeight), Game1.textColor);
        }
    }

    private static int GetCenteredTextY(Rectangle area, int lineHeight)
    {
        return area.Y + Math.Max(0, (area.Height - lineHeight) / 2);
    }
}
