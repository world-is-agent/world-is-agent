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
    private const int MenuWidth = 1100;
    private const int Margin = 24;
    private const int OptionHeight = 64;
    private const int TextInputHeight = 72;
    private const int ButtonWidth = 180;
    private const int ButtonHeight = 56;

    private readonly PresentDialogueInput input;
    private readonly Action<PlayerDialogueSubmission> onSubmitted;
    private readonly Action onAbandoned;
    private readonly List<OptionButton> optionButtons = new();
    private readonly Rectangle textArea;
    private readonly Rectangle closeButton;
    private readonly Rectangle submitButton;
    private readonly DialogueFreeTextInput? textInput;
    private bool submitted;
    private bool closedWithoutSubmission;
    private bool abandonmentNotified;

    public DialogueInteractionMenu(
        string npcEntityId,
        string conversationId,
        PresentDialogueInput input,
        Action<PlayerDialogueSubmission> onSubmitted,
        Action onAbandoned
    )
    {
        this.NpcEntityId = npcEntityId;
        this.ConversationId = conversationId;
        this.input = input;
        this.onSubmitted = onSubmitted;
        this.onAbandoned = onAbandoned;

        int width = Math.Min(MenuWidth, Math.Max(720, Game1.uiViewport.Width - 128));
        int textHeight = Math.Max(96, MeasureWrappedHeight(input.Text, Game1.dialogueFont, width - Margin * 2));
        int optionsHeight = input.ReplyOptions.Count * (OptionHeight + 12);
        int inputHeight = input.AllowFreeText ? TextInputHeight + ButtonHeight + 24 : 0;
        int height = Math.Min(Game1.uiViewport.Height - 96, Margin * 3 + textHeight + optionsHeight + inputHeight + ButtonHeight);

        this.xPositionOnScreen = (Game1.uiViewport.Width - width) / 2;
        this.yPositionOnScreen = (Game1.uiViewport.Height - height) / 2;
        this.width = width;
        this.height = height;
        this.textArea = new Rectangle(this.xPositionOnScreen + Margin, this.yPositionOnScreen + Margin, this.width - Margin * 2, textHeight);

        int currentY = this.yPositionOnScreen + Margin + textHeight + 16;
        for (int i = 0; i < input.ReplyOptions.Count; i++)
        {
            this.optionButtons.Add(new OptionButton(
                i,
                input.ReplyOptions[i],
                new Rectangle(this.xPositionOnScreen + Margin, currentY, width - Margin * 2, OptionHeight)
            ));
            currentY += OptionHeight + 12;
        }

        if (input.AllowFreeText)
        {
            this.textInput = new DialogueFreeTextInput(ConversationStateStore.MaxLineTextChars);
            this.textInput.Bounds = new Rectangle(this.xPositionOnScreen + Margin, currentY, width - Margin * 2, TextInputHeight);
            Game1.keyboardDispatcher.Subscriber = this.textInput;
            currentY += TextInputHeight + 12;
        }

        this.submitButton = new Rectangle(this.xPositionOnScreen + width - Margin - ButtonWidth, currentY, ButtonWidth, ButtonHeight);
        this.closeButton = new Rectangle(this.xPositionOnScreen + Margin, currentY, ButtonWidth, ButtonHeight);
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
        foreach (OptionButton option in this.optionButtons)
        {
            if (!option.Bounds.Contains(x, y))
                continue;

            this.Submit("option", option.Text, option.Index, "dialogue_option");
            return;
        }

        if (this.textInput is not null && this.submitButton.Contains(x, y))
        {
            string text = this.textInput.Text.Trim();
            if (text.Length > 0)
                this.Submit("free_text", text, null, "dialogue_free_text");
            return;
        }

        if (this.closeButton.Contains(x, y))
            this.exitThisMenu();
    }

    public override void receiveKeyPress(Keys key)
    {
        if (key == Keys.Escape)
        {
            this.exitThisMenu();
            return;
        }

        if (this.textInput is not null)
        {
            if (key == Keys.Enter)
            {
                string text = this.textInput.Text.Trim();
                if (text.Length > 0)
                    this.Submit("free_text", text, null, "dialogue_free_text");
                return;
            }

            this.textInput.RecieveSpecialInput(key);
        }
    }

    public override void draw(SpriteBatch b)
    {
        b.Draw(Game1.fadeToBlackRect, Game1.graphics.GraphicsDevice.Viewport.Bounds, Color.Black * 0.35f);
        Game1.drawDialogueBox(this.xPositionOnScreen, this.yPositionOnScreen, this.width, this.height, false, true);

        DrawWrappedText(b, this.input.Text, Game1.dialogueFont, Game1.textColor, this.textArea);

        foreach (OptionButton option in this.optionButtons)
        {
            IClickableMenu.drawTextureBox(b, option.Bounds.X, option.Bounds.Y, option.Bounds.Width, option.Bounds.Height, Color.White);
            b.DrawString(Game1.smallFont, option.Text, new Vector2(option.Bounds.X + 20, option.Bounds.Y + 18), Game1.textColor);
        }

        if (this.textInput is not null)
            this.textInput.Draw(b);

        DrawButton(b, this.closeButton, "Close");
        if (this.textInput is not null)
            DrawButton(b, this.submitButton, "Send");

        drawMouse(b);
    }

    protected override void cleanupBeforeExit()
    {
        if (Game1.keyboardDispatcher.Subscriber == this.textInput)
            Game1.keyboardDispatcher.Subscriber = null;

        if (!this.submitted && !this.closedWithoutSubmission)
            this.NotifyAbandoned();

        base.cleanupBeforeExit();
    }

    private void Submit(string inputKind, string text, int? selectedOptionIndex, string trigger)
    {
        this.submitted = true;
        this.onSubmitted(new PlayerDialogueSubmission(this.ConversationId, inputKind, text, selectedOptionIndex, trigger));
        this.exitThisMenu();
    }

    private void NotifyAbandoned()
    {
        if (this.abandonmentNotified)
            return;

        this.abandonmentNotified = true;
        this.onAbandoned();
    }

    private static void DrawButton(SpriteBatch b, Rectangle bounds, string label)
    {
        IClickableMenu.drawTextureBox(b, bounds.X, bounds.Y, bounds.Width, bounds.Height, Color.White);
        Vector2 size = Game1.smallFont.MeasureString(label);
        Vector2 position = new(bounds.X + (bounds.Width - size.X) / 2, bounds.Y + (bounds.Height - size.Y) / 2);
        b.DrawString(Game1.smallFont, label, position, Game1.textColor);
    }

    private static int MeasureWrappedHeight(string text, SpriteFont font, int maxWidth)
    {
        return WrapText(text, font, maxWidth).Count * (int)font.MeasureString("A").Y;
    }

    private static void DrawWrappedText(SpriteBatch b, string text, SpriteFont font, Color color, Rectangle area)
    {
        int lineHeight = (int)font.MeasureString("A").Y;
        int y = area.Y;
        foreach (string line in WrapText(text, font, area.Width))
        {
            if (y + lineHeight > area.Bottom)
                break;

            b.DrawString(font, line, new Vector2(area.X, y), color);
            y += lineHeight;
        }
    }

    private static IReadOnlyList<string> WrapText(string text, SpriteFont font, int maxWidth)
    {
        var lines = new List<string>();
        string current = string.Empty;
        foreach (string word in (text ?? string.Empty).Split(' ', StringSplitOptions.RemoveEmptyEntries))
        {
            string candidate = string.IsNullOrEmpty(current) ? word : $"{current} {word}";
            if (font.MeasureString(candidate).X <= maxWidth)
            {
                current = candidate;
                continue;
            }

            if (!string.IsNullOrEmpty(current))
                lines.Add(current);

            current = word;
        }

        if (!string.IsNullOrEmpty(current))
            lines.Add(current);

        if (lines.Count == 0)
            lines.Add(string.Empty);

        return lines;
    }

    private sealed record OptionButton(int Index, string Text, Rectangle Bounds);

    private sealed class DialogueFreeTextInput : IKeyboardSubscriber
    {
        private readonly int characterLimit;

        public DialogueFreeTextInput(int characterLimit)
        {
            this.characterLimit = characterLimit;
        }

        public Rectangle Bounds { get; set; }
        public string Text { get; private set; } = string.Empty;
        public bool Selected { get; set; } = true;

        public void Draw(SpriteBatch b)
        {
            IClickableMenu.drawTextureBox(b, this.Bounds.X, this.Bounds.Y, this.Bounds.Width, this.Bounds.Height, Color.White);
            b.DrawString(Game1.smallFont, this.Text, new Vector2(this.Bounds.X + 16, this.Bounds.Y + 18), Game1.textColor);
        }

        public void RecieveTextInput(char inputChar)
        {
            if (inputChar == '\b')
            {
                if (this.Text.Length > 0)
                    this.Text = this.Text[..^1];
                return;
            }

            if (char.IsControl(inputChar) || this.Text.Length >= this.characterLimit)
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
            this.RecieveTextInput(command);
        }

        public void RecieveSpecialInput(Keys key)
        {
            if (key == Keys.Back && this.Text.Length > 0)
                this.Text = this.Text[..^1];
        }
    }
}
