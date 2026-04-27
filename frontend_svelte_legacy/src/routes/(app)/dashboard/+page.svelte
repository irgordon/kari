<script lang="ts">
  import { fade, slide } from "svelte/transition";
  import type { PageData } from "./$types";
  import {
    Activity,
    ShieldAlert,
    Server,
    Cpu,
    Database,
    Clock,
    AlertTriangle,
    Info,
    ShieldCheck,
  } from "lucide-svelte";

  export let data: PageData;

  // Reactive mapping from server data
  $: stats = data.stats;
  $: alerts = data.alerts;

  // Helper: Format uptime into a human-readable string
  function formatUptime(seconds: number) {
    const days = Math.floor(seconds / (24 * 3600));
    const hrs = Math.floor((seconds % (24 * 3600)) / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    return days > 0 ? `${days}d ${hrs}h` : `${hrs}h ${mins}m`;
  }

  // Helper: Severity styling for alerts (updated for Dark Bento aesthetic)
  const alertStyles = {
    critical:
      "bg-red-500/10 ring-1 ring-red-500/30 text-red-400 shadow-[0_0_15px_rgba(239,68,68,0.15)] icon-red-400",
    warning:
      "bg-amber-500/10 ring-1 ring-amber-500/30 text-amber-400 shadow-[0_0_15px_rgba(245,158,11,0.15)] icon-amber-400",
    info: "bg-blue-500/10 ring-1 ring-blue-500/30 text-blue-400 shadow-[0_0_15px_rgba(59,130,246,0.15)] icon-blue-400",
  };
</script>

<div class="space-y-8 pb-10" in:fade={{ duration: 200 }}>
  <!-- Top Bento Grid: Metrics -->
  <section class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
    <!-- Active Jails Card -->
    <div
      class="bg-slate-900/50 backdrop-blur-md p-6 ring-1 ring-white/10 rounded-2xl shadow-lg flex items-center gap-5 hover:bg-slate-800/50 transition-colors"
    >
      <div
        class="p-3.5 bg-indigo-500/20 text-indigo-400 rounded-xl ring-1 ring-indigo-500/30 shadow-[0_0_20px_rgba(99,102,241,0.2)]"
      >
        <Server size={24} />
      </div>
      <div>
        <p
          class="text-[10px] font-bold text-slate-400 uppercase tracking-widest mb-1"
        >
          Active Jails
        </p>
        <h3 class="text-3xl font-bold tracking-tight text-white">
          {stats.active_jails}
        </h3>
      </div>
    </div>

    <!-- CPU Load Card -->
    <div
      class="bg-slate-900/50 backdrop-blur-md p-6 ring-1 ring-white/10 rounded-2xl shadow-lg flex flex-col justify-center hover:bg-slate-800/50 transition-colors"
    >
      <div class="flex items-center justify-between mb-2">
        <p
          class="text-[10px] font-bold text-slate-400 uppercase tracking-widest"
        >
          CPU Load
        </p>
        <Cpu size={16} class="text-slate-500" />
      </div>
      <h3 class="text-3xl font-bold tracking-tight text-white">
        {stats.cpu_usage}%
      </h3>
      <!-- Smooth Pill Visualization -->
      <div
        class="w-full bg-slate-800 h-2 rounded-full mt-3 overflow-hidden ring-1 ring-white/5 shadow-inner"
      >
        <div
          class="bg-gradient-to-r from-indigo-500 to-violet-500 h-full rounded-full transition-all duration-500 shadow-[0_0_10px_rgba(99,102,241,0.5)]"
          style="width: {stats.cpu_usage}%"
        ></div>
      </div>
    </div>

    <!-- Memory Load Card -->
    <div
      class="bg-slate-900/50 backdrop-blur-md p-6 ring-1 ring-white/10 rounded-2xl shadow-lg flex flex-col justify-center hover:bg-slate-800/50 transition-colors"
    >
      <div class="flex items-center justify-between mb-2">
        <p
          class="text-[10px] font-bold text-slate-400 uppercase tracking-widest"
        >
          Memory
        </p>
        <Database size={16} class="text-slate-500" />
      </div>
      <h3 class="text-3xl font-bold tracking-tight text-white">
        {stats.ram_usage}%
      </h3>
      <!-- Smooth Pill Visualization -->
      <div
        class="w-full bg-slate-800 h-2 rounded-full mt-3 overflow-hidden ring-1 ring-white/5 shadow-inner"
      >
        <div
          class="bg-gradient-to-r from-indigo-500 to-violet-500 h-full rounded-full transition-all duration-500 shadow-[0_0_10px_rgba(99,102,241,0.5)]"
          style="width: {stats.ram_usage}%"
        ></div>
      </div>
    </div>

    <!-- Uptime Card -->
    <div
      class="bg-slate-900/50 backdrop-blur-md p-6 ring-1 ring-white/10 rounded-2xl shadow-lg flex items-center gap-5 hover:bg-slate-800/50 transition-colors"
    >
      <div
        class="p-3.5 bg-emerald-500/20 text-emerald-400 rounded-xl ring-1 ring-emerald-500/30 shadow-[0_0_20px_rgba(16,185,129,0.2)]"
      >
        <Clock size={24} />
      </div>
      <div>
        <p
          class="text-[10px] font-bold text-slate-400 uppercase tracking-widest mb-1"
        >
          System Uptime
        </p>
        <h3
          class="text-2xl font-bold tracking-tight text-white truncate max-w-[120px]"
        >
          {formatUptime(stats.uptime_seconds)}
        </h3>
      </div>
    </div>
  </section>

  <!-- Lower Bento Grid: Alerts & Status -->
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <!-- Priority Alerts Column -->
    <section class="lg:col-span-2 space-y-5">
      <header class="flex items-center justify-between px-1">
        <h2
          class="text-sm font-semibold text-slate-400 uppercase tracking-widest flex items-center gap-2"
        >
          <ShieldAlert size={16} class="text-slate-500" />
          Priority Alerts
        </h2>
        <span
          class="text-[10px] font-mono text-slate-500 bg-slate-900/50 px-2 py-1 rounded-md ring-1 ring-white/5"
        >
          Snapshot: {new Date(data.snapshotAt).toLocaleTimeString()}
        </span>
      </header>

      <div class="space-y-4">
        {#each alerts as alert (alert.id)}
          <div
            transition:slide
            class="flex gap-4 p-5 rounded-2xl shadow-lg backdrop-blur-md {alertStyles[
              alert.severity
            ].split(' icon')[0]}"
          >
            <div class="shrink-0 mt-0.5">
              {#if alert.severity === "critical"}
                <AlertTriangle
                  size={20}
                  class="text-red-400 drop-shadow-[0_0_8px_rgba(239,68,68,0.5)]"
                />
              {:else if alert.severity === "warning"}
                <AlertTriangle
                  size={20}
                  class="text-amber-400 drop-shadow-[0_0_8px_rgba(245,158,11,0.5)]"
                />
              {:else}
                <Info
                  size={20}
                  class="text-blue-400 drop-shadow-[0_0_8px_rgba(59,130,246,0.5)]"
                />
              {/if}
            </div>
            <div class="flex-1">
              <div class="flex justify-between items-start mb-1">
                <span
                  class="text-[10px] font-bold uppercase tracking-widest opacity-80"
                  >{alert.category}</span
                >
                <span class="text-[10px] font-mono opacity-50"
                  >{new Date(alert.created_at).toLocaleTimeString()}</span
                >
              </div>
              <p class="text-sm font-medium leading-relaxed">{alert.message}</p>
            </div>
          </div>
        {:else}
          <div
            class="flex flex-col items-center justify-center py-24 bg-slate-900/30 backdrop-blur-md rounded-2xl ring-1 ring-white/5 border border-dashed border-white/10 text-slate-500"
          >
            <ShieldCheck size={48} class="opacity-30 mb-5" />
            <p class="text-sm font-medium text-slate-400">
              All systems operational.
            </p>
            <p class="text-[10px] uppercase tracking-widest opacity-50 mt-1">
              No unresolved high-priority threats.
            </p>
          </div>
        {/each}
      </div>
    </section>

    <!-- Muscle Status Column -->
    <section class="space-y-5">
      <h2
        class="text-sm font-semibold text-slate-400 uppercase tracking-widest flex items-center gap-2 px-1"
      >
        <Activity size={16} class="text-slate-500" />
        Muscle Status
      </h2>

      <!-- Deep Glow Bento Card -->
      <div
        class="relative bg-slate-900/60 backdrop-blur-xl rounded-2xl p-7 shadow-2xl ring-1 ring-white/10 overflow-hidden group"
      >
        <!-- Ambient Accent Glow behind the card contents -->
        <div
          class="absolute -top-24 -right-24 w-48 h-48 bg-indigo-500/20 blur-3xl rounded-full pointer-events-none group-hover:bg-indigo-500/30 transition-all duration-700"
        ></div>

        <div class="relative z-10 space-y-8">
          <div>
            <p
              class="text-[10px] font-bold text-slate-400 uppercase tracking-widest mb-4"
            >
              Orchestration Integrity
            </p>
            <div
              class="flex items-center gap-4 bg-slate-950/50 p-3 rounded-xl ring-1 ring-white/5 w-fit"
            >
              <div class="relative flex h-3 w-3">
                <span
                  class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
                ></span>
                <span
                  class="relative inline-flex rounded-full h-3 w-3 bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.5)]"
                ></span>
              </div>
              <span class="text-sm font-bold text-white tracking-wide"
                >Encrypted Link Active</span
              >
            </div>
          </div>

          <div class="pt-6 border-t border-white/5">
            <p class="text-[11px] text-slate-400 leading-relaxed font-medium">
              The Karı Muscle is currently enforcing <strong
                class="text-indigo-300 font-bold">{stats.active_jails}</strong
              > jail boundaries. Resource utilization remains strictly within configured
              SLA safety margins.
            </p>
          </div>

          <a
            href="/applications"
            class="mt-4 flex items-center justify-center w-full py-3.5 bg-indigo-500/10 hover:bg-indigo-500/20 text-indigo-300 rounded-xl text-xs font-bold transition-all ring-1 ring-indigo-500/30 shadow-[0_0_15px_rgba(99,102,241,0.1)] hover:shadow-[0_0_25px_rgba(99,102,241,0.2)] uppercase tracking-wider"
          >
            Manage Infrastructure
          </a>
        </div>
      </div>
    </section>
  </div>
</div>
