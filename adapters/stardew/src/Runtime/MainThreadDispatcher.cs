using System;
using System.Collections.Concurrent;
using StardewModdingAPI;

namespace GameAgent.Stardew.Runtime;

public sealed class MainThreadDispatcher
{
    private readonly ConcurrentQueue<Action> workItems = new();
    private readonly IMonitor monitor;

    public MainThreadDispatcher(IMonitor monitor)
    {
        this.monitor = monitor;
    }

    public void Enqueue(Action work)
    {
        this.workItems.Enqueue(work);
    }

    public void Drain()
    {
        while (this.workItems.TryDequeue(out Action? work))
        {
            try
            {
                work();
            }
            catch (Exception ex)
            {
                this.monitor.Log($"GameAgent main-thread task failed: {ex}", LogLevel.Error);
            }
        }
    }
}
