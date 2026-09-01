using Microsoft.Xna.Framework;
using Microsoft.Xna.Framework.Graphics;
using Microsoft.Xna.Framework.Input;
using StardewValley;
using StardewValley.Menus;

namespace GameAgent.Stardew.Dialogue;

public sealed class DialogueWaitingMenu : IClickableMenu
{
    private const int Margin = 24;
    private readonly string message;
    private int animationFrame;
    private float animationTimer;

    public DialogueWaitingMenu(string npcEntityId, string message = "Thinking") : base()
    {
        this.NpcEntityId = npcEntityId;
        this.message = string.IsNullOrWhiteSpace(message) ? "Thinking" : message;

        Vector2 messageSize = Game1.dialogueFont.MeasureString($"{this.message}...");
        this.width = (int)messageSize.X + 6 * Margin;
        this.height = (int)messageSize.Y + 6 * Margin;
        this.xPositionOnScreen = (Game1.uiViewport.Width - this.width) / 2;
        this.yPositionOnScreen = (Game1.uiViewport.Height - this.height) / 2;
    }

    public string NpcEntityId { get; }

    public void Close()
    {
        this.exitThisMenu();
    }

    public override void update(GameTime time)
    {
        base.update(time);
        this.animationTimer += (float)time.ElapsedGameTime.TotalMilliseconds;
        if (this.animationTimer < 500f)
            return;

        this.animationFrame = (this.animationFrame + 1) % 4;
        this.animationTimer = 0f;
    }

    public override void draw(SpriteBatch b)
    {
        b.Draw(Game1.fadeToBlackRect, Game1.graphics.GraphicsDevice.Viewport.Bounds, Color.Black * 0.3f);
        Game1.drawDialogueBox(this.xPositionOnScreen, this.yPositionOnScreen, this.width, this.height, false, true);

        string animatedMessage = $"{this.message}{new string('.', this.animationFrame)}";
        Vector2 size = Game1.dialogueFont.MeasureString(animatedMessage);
        Vector2 position = new(
            this.xPositionOnScreen + (this.width - size.X) / 2,
            this.yPositionOnScreen + (this.height - size.Y) / 2
        );
        b.DrawString(Game1.dialogueFont, animatedMessage, position, Game1.textColor);

        drawMouse(b);
    }

    public override void receiveLeftClick(int x, int y, bool playSound = true)
    {
    }

    public override void receiveKeyPress(Keys key)
    {
    }

    public override bool overrideSnappyMenuCursorMovementBan()
    {
        return true;
    }
}
