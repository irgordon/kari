<script lang="ts">
  import { page } from "$app/stores";
  import { fade } from "svelte/transition";
  import type { LayoutData } from "./$types";
  import {
    LayoutDashboard,
    Boxes,
    Globe,
    Bell,
    Settings,
    LogOut,
    User as UserIcon,
  } from "lucide-svelte";

  // Data securely provided by +layout.server.ts
  export let data: LayoutData;
  $: user = data.user;

  // Standardized navigation map using Lucide components
  const navItems = [
    { name: "Dashboard", path: "/dashboard", icon: LayoutDashboard },
    { name: "Applications", path: "/applications", icon: Boxes },
    { name: "Domains", path: "/domains", icon: Globe },
    { name: "Action Center", path: "/action-center", icon: Bell },
    { name: "Settings", path: "/settings", icon: Settings },
  ];

  // 🛡️ Logic: Precision active state detection
  $: currentPath = $page.url.pathname;
  const isActive = (path: string) => {
    if (path === "/dashboard" && currentPath === "/") return true;
    return currentPath.startsWith(path);
  };
</script>

<div
  class="flex h-screen w-full bg-slate-950 font-body antialiased text-slate-200 overflow-hidden"
>
  <!-- Slim, floating left-hand nav -->
  <aside
    class="w-20 my-4 ml-4 flex flex-col items-center bg-slate-900/60 backdrop-blur-xl ring-1 ring-white/10 rounded-2xl shadow-lg shrink-0 py-6 z-50"
  >
    <div class="mb-8 flex flex-col items-center">
      <div
        class="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 flex items-center justify-center text-white font-sans font-bold text-xl shadow-[0_0_15px_rgba(99,102,241,0.4)]"
      >
        K
      </div>
    </div>

    <nav
      class="flex-1 w-full px-3 flex flex-col items-center gap-4 overflow-y-auto"
    >
      {#each navItems as item}
        <a
          href={item.path}
          class="relative flex items-center justify-center w-12 h-12 rounded-xl transition-all duration-300 group
                           {isActive(item.path)
            ? 'bg-indigo-500/20 text-indigo-400 ring-1 ring-indigo-500/50 shadow-[0_0_15px_rgba(99,102,241,0.2)]'
            : 'text-slate-400 hover:bg-white/5 hover:text-white'}"
        >
          <svelte:component
            this={item.icon}
            size={22}
            strokeWidth={isActive(item.path) ? 2.5 : 2}
          />
          <!-- Animated Tooltip -->
          <div
            class="absolute left-16 px-3 py-1.5 bg-slate-800 text-white text-xs font-semibold rounded-lg opacity-0 -translate-x-2 pointer-events-none group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300 shadow-xl whitespace-nowrap ring-1 ring-white/10 z-50"
          >
            {item.name}
          </div>
        </a>
      {/each}
    </nav>

    {#if user}
      <div
        class="mt-auto w-full px-3 pt-6 border-t border-white/10 flex flex-col items-center gap-4"
      >
        <div class="relative group cursor-pointer">
          <div
            class="w-10 h-10 rounded-full bg-slate-800 ring-1 ring-white/10 flex items-center justify-center text-slate-300 hover:text-white transition-colors"
          >
            <UserIcon size={20} />
          </div>
          <div
            class="absolute left-14 bottom-0 px-3 py-2 bg-slate-800 text-white text-xs rounded-lg opacity-0 -translate-x-2 pointer-events-none group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300 shadow-xl whitespace-nowrap ring-1 ring-white/10 z-50"
          >
            <p class="font-bold">{user.id.split("-")[0]}</p>
            <p
              class="text-[10px] text-indigo-400 uppercase tracking-wider mt-0.5"
            >
              {user.role || "Operator"}
            </p>
          </div>
        </div>

        <form method="POST" action="/logout" class="w-full">
          <button
            type="submit"
            class="w-full flex items-center justify-center h-10 rounded-xl text-slate-400 hover:text-red-400 hover:bg-red-500/10 transition-all group"
            title="Sign out"
          >
            <LogOut
              size={18}
              class="group-hover:translate-x-0.5 transition-transform"
            />
          </button>
        </form>
      </div>
    {/if}
  </aside>

  <main class="flex-1 flex flex-col min-w-0 overflow-hidden py-4 px-6 relative">
    <!-- Floating Header -->
    <header
      class="h-16 mb-6 flex items-center justify-between px-6 bg-slate-900/40 backdrop-blur-md ring-1 ring-white/10 rounded-2xl shadow-sm shrink-0"
    >
      <h1
        class="font-sans font-semibold text-slate-300 text-sm uppercase tracking-widest"
      >
        {currentPath === "/"
          ? "Dashboard"
          : currentPath.split("/")[1].replace("-", " ")}
      </h1>

      <div
        class="flex items-center gap-3 px-3 py-1.5 rounded-full bg-slate-950/50 ring-1 ring-white/5"
      >
        <div class="relative flex h-2 w-2">
          <span
            class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
          ></span>
          <span class="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"
          ></span>
        </div>
        <span
          class="text-[10px] font-bold text-slate-400 uppercase tracking-widest"
          >Muscle Link: Active</span
        >
      </div>
    </header>

    <div class="flex-1 overflow-y-auto rounded-2xl">
      <div class="max-w-7xl mx-auto h-full">
        {#key currentPath}
          <div in:fade={{ duration: 150 }} class="h-full">
            <slot />
          </div>
        {/key}
      </div>
    </div>
  </main>
</div>
