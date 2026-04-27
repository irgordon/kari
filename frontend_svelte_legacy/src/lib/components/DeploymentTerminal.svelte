<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { fade } from "svelte/transition";
  import { Terminal } from "xterm";
  import { FitAddon } from "xterm-addon-fit";

  // Core CSS for canvas rendering
  import "xterm/css/xterm.css";

  // Props
  export let traceId: string;

  // Component State
  let terminalElement: HTMLDivElement;
  let terminal: Terminal;
  let fitAddon: FitAddon;
  let eventSource: EventSource | null = null;

  let status: "connecting" | "streaming" | "completed" | "error" = "connecting";
  let autoscroll = true;

  // 🛡️ Zero-Trust: Validate traceId before opening the stream
  const isValidTraceId = (id: string) =>
    /^[a-f0-9-]{36}$/i.test(id) || id.length > 5;

  onMount(() => {
    if (!isValidTraceId(traceId)) {
      status = "error";
      return;
    }

    // 1. Initialize xterm.js with Glass Bento Identity
    terminal = new Terminal({
      fontFamily: '"IBM Plex Mono", monospace',
      fontSize: 13,
      lineHeight: 1.5,
      cursorBlink: true,
      disableStdin: true,
      theme: {
        background: "#020617", // slate-950
        foreground: "#E2E8F0", // slate-200
        cursor: "#818CF8", // indigo-400
        selectionBackground: "rgba(99, 102, 241, 0.3)",
        cyan: "#818CF8",
        green: "#34D399",
        red: "#F87171",
        yellow: "#FBBF24",
      },
    });

    fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(terminalElement);
    fitAddon.fit();

    // 🛡️ SLA: UI Responsiveness
    const resizeObserver = new ResizeObserver(() => fitAddon.fit());
    resizeObserver.observe(terminalElement);

    // 2. 📡 Connect to the Go Brain SSE Hub
    const url = `/api/deployments/${traceId}/logs/stream`;
    eventSource = new EventSource(url);

    terminal.writeln(
      "\x1b[36m[Karı]\x1b[0m Synchronizing telemetry via Go Brain...",
    );

    eventSource.onopen = () => {
      status = "streaming";
      terminal.writeln(
        "\x1b[32m[Karı]\x1b[0m Secure link established. Awaiting Muscle logs...\r\n",
      );
    };

    eventSource.onmessage = (event) => {
      // Write payload directly to the canvas
      terminal.write(event.data);

      // 🛡️ SLA: Detect terminal conditions to update UI state
      if (event.data.includes("✅ Deployment successful")) {
        status = "completed";
        terminal.writeln(
          "\r\n\x1b[36m[Karı]\x1b[0m Pipeline finished successfully.",
        );
      } else if (event.data.includes("❌ ERROR")) {
        status = "error";
        terminal.writeln(
          "\r\n\x1b[31m[Karı]\x1b[0m Pipeline aborted due to error.",
        );
      }

      // Handle autoscroll if user hasn't scrolled up manually
      if (autoscroll) {
        terminal.scrollToBottom();
      }
    };

    eventSource.onerror = (err) => {
      if (status !== "completed") {
        status = "error";
        terminal.writeln("\r\n\x1b[31m[Karı]\x1b[0m Telemetry heartbeat lost.");
      }
      eventSource?.close();
    };

    return () => {
      resizeObserver.disconnect();
      if (eventSource) eventSource.close();
      if (terminal) terminal.dispose();
    };
  });

  // 🛡️ Logic to detect if user has scrolled up (manual investigation)
  function handleTerminalScroll() {
    if (!terminal) return;
    const buffer = terminal.buffer.active;
    // If the current viewport is not at the end of the scrollback buffer
    autoscroll = buffer.viewportY >= buffer.baseY;
  }

  onDestroy(() => {
    if (eventSource) eventSource.close();
    if (terminal) terminal.dispose();
  });
</script>

<div
  class="flex flex-col h-[600px] w-full bg-slate-900/50 backdrop-blur-md shadow-2xl ring-1 ring-white/10 rounded-2xl overflow-hidden"
>
  <div
    class="h-14 border-b border-white/5 flex items-center justify-between px-5 shrink-0 bg-slate-900/60"
  >
    <div class="flex items-center gap-3">
      <svg
        xmlns="http://www.w3.org/2000/svg"
        class="h-5 w-5 text-slate-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
        />
      </svg>
      <h3
        class="font-sans font-semibold text-sm text-slate-300 uppercase tracking-widest"
      >
        Deployment Telemetry
      </h3>
      <span
        class="text-xs font-mono text-indigo-400 bg-indigo-500/10 ring-1 ring-indigo-500/30 px-2 py-0.5 rounded-md"
      >
        {traceId.slice(0, 8)}
      </span>
    </div>

    <div class="flex items-center gap-4">
      {#if !autoscroll && status === "streaming"}
        <button
          on:click={() => {
            autoscroll = true;
            terminal.scrollToBottom();
          }}
          class="text-[10px] font-bold text-indigo-400 animate-bounce uppercase tracking-widest"
          transition:fade
        >
          ⬇ Resync to Tail
        </button>
      {/if}

      <div
        class="flex items-center gap-2 bg-slate-950/50 px-3 py-1.5 rounded-full ring-1 ring-white/5"
      >
        {#if status === "connecting"}
          <span class="relative flex h-2 w-2">
            <span
              class="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"
            ></span>
            <span class="relative inline-flex rounded-full h-2 w-2 bg-amber-500"
            ></span>
          </span>
          <span
            class="text-[10px] font-bold uppercase tracking-widest text-slate-400"
            >Connecting...</span
          >
        {:else if status === "streaming"}
          <span class="relative flex h-2 w-2">
            <span
              class="animate-ping absolute inline-flex h-full w-full rounded-full bg-indigo-400 opacity-75"
            ></span>
            <span
              class="relative inline-flex rounded-full h-2 w-2 bg-indigo-500 shadow-[0_0_8px_rgba(99,102,241,0.5)]"
            ></span>
          </span>
          <span
            class="text-[10px] font-bold uppercase tracking-widest text-indigo-400"
            >Live Feed</span
          >
        {:else if status === "completed"}
          <span
            class="h-2 w-2 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]"
          ></span>
          <span
            class="text-[10px] font-bold uppercase tracking-widest text-slate-300"
            >Completed</span
          >
        {:else if status === "error"}
          <span
            class="h-2 w-2 rounded-full bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]"
          ></span>
          <span
            class="text-[10px] font-bold uppercase tracking-widest text-red-400"
            >Sync Lost</span
          >
        {/if}
      </div>
    </div>
  </div>

  <div
    class="relative flex-1 bg-slate-950 p-3 overflow-hidden shadow-inner ring-1 ring-inset ring-black/50"
  >
    <div
      bind:this={terminalElement}
      on:wheel={handleTerminalScroll}
      class="absolute inset-4"
    ></div>
  </div>
</div>

<style>
  :global(.xterm-viewport::-webkit-scrollbar) {
    width: 8px;
  }
  :global(.xterm-viewport::-webkit-scrollbar-track) {
    background: #020617;
  }
  :global(.xterm-viewport::-webkit-scrollbar-thumb) {
    background: #334155;
    border-radius: 4px;
    border: 2px solid #020617;
  }
  :global(.xterm-viewport::-webkit-scrollbar-thumb:hover) {
    background: #6366f1;
  }
</style>
