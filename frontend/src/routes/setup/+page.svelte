<script lang="ts">
  import { onMount } from "svelte";
  import { fade, fly, slide } from "svelte/transition";
  import { cubicOut } from "svelte/easing";
  import {
    CheckCircle,
    AlertCircle,
    ShieldCheck,
    Database,
    HardDrive,
    Key,
    Loader2,
    Copy,
    AlertTriangle,
    Zap,
    Lock,
  } from "lucide-svelte";

  // 🛡️ State
  let step = 1;
  let loading = false;
  let setupToken = "";
  let copied = false;

  // Test Results (Green Lights)
  let muscleStatus: {
    healthy: boolean;
    version?: string;
    cpu?: string;
    ram_mb?: number;
    error?: string;
  } | null = null;
  let dbStatus: {
    healthy: boolean;
    host?: string;
    port?: string;
    error?: string;
  } | null = null;

  // Form State
  let adminEmail = "";
  let adminPassword = "";
  let confirmPassword = "";
  let dbUrl = "";
  let appDomain = "";

  // Security State
  let masterKey: {
    hex_key: string;
    recovery_phrase: string;
    word_count: number;
  } | null = null;

  // Validation
  $: passwordValid = adminPassword.length >= 12;
  $: passwordsMatch =
    adminPassword === confirmPassword && confirmPassword.length > 0;
  $: emailValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(adminEmail);
  $: domainValid = appDomain.length > 3 && appDomain.includes(".");
  $: securityReady =
    passwordValid && passwordsMatch && emailValid && masterKey !== null;

  onMount(() => {
    const urlParams = new URLSearchParams(window.location.search);
    setupToken = urlParams.get("token") || "";
    testMuscle();
  });

  function authHeaders(): Record<string, string> {
    return { "X-Setup-Token": setupToken, "Content-Type": "application/json" };
  }

  async function testMuscle() {
    loading = true;
    try {
      const res = await fetch(`/api/v1/setup/test-muscle?token=${setupToken}`);
      muscleStatus = await res.json();
    } catch {
      muscleStatus = { healthy: false, error: "Brain cannot reach Muscle UDS" };
    } finally {
      loading = false;
    }
  }

  async function testDB() {
    loading = true;
    dbStatus = null;
    try {
      const res = await fetch("/api/v1/setup/test-db", {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({ database_url: dbUrl }),
      });
      dbStatus = await res.json();
    } catch {
      dbStatus = { healthy: false, error: "Network error during DB probe" };
    } finally {
      loading = false;
    }
  }

  async function generateKey() {
    loading = true;
    try {
      const res = await fetch("/api/v1/setup/generate-key", {
        method: "POST",
        headers: authHeaders(),
      });
      masterKey = await res.json();
    } catch {
      alert("Failed to generate master key");
    } finally {
      loading = false;
    }
  }

  async function finalize() {
    if (!masterKey || !securityReady) return;
    loading = true;
    try {
      const res = await fetch("/api/v1/setup/finalize", {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify({
          admin_email: adminEmail,
          admin_password: adminPassword,
          database_url: dbUrl,
          app_domain: appDomain,
          master_key_hex: masterKey.hex_key,
        }),
      });
      const data = await res.json();
      if (res.ok) {
        step = 5; // Final lockdown screen
      } else {
        alert(data.message || "Finalization failed");
      }
    } catch {
      alert("Network error during finalization");
    } finally {
      loading = false;
    }
  }

  function copyPhrase() {
    if (!masterKey) return;
    navigator.clipboard.writeText(masterKey.recovery_phrase);
    copied = true;
    setTimeout(() => (copied = false), 3000);
  }

  const steps = [
    { num: 1, label: "Hardware", icon: HardDrive },
    { num: 2, label: "Database", icon: Database },
    { num: 3, label: "Security", icon: ShieldCheck },
    { num: 4, label: "Recovery", icon: Key },
  ];
</script>

<main class="min-h-screen bg-slate-950 text-slate-200 font-sans">
  <!-- 🛡️ Top Bar -->
  <div
    class="bg-slate-900/40 backdrop-blur-xl border-b border-white/5 px-6 py-4 sticky top-0 z-50 ring-1 ring-white/10"
  >
    <div class="max-w-2xl mx-auto flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div
          class="w-8 h-8 bg-gradient-to-br from-indigo-500 to-violet-600 rounded-lg flex items-center justify-center"
        >
          <ShieldCheck size={18} class="text-white" />
        </div>
        <h1 class="text-lg font-bold tracking-tight">Karı Setup Wizard</h1>
      </div>
      <span class="text-xs text-slate-500 font-mono"
        >Zero-Trust Configuration</span
      >
    </div>
  </div>

  <div class="max-w-2xl mx-auto px-6 py-10">
    <!-- Step Progress -->
    {#if step <= 4}
      <div class="flex items-center justify-between mb-12 px-4">
        {#each steps as s, i}
          <div
            class="flex items-center gap-2 {step >= s.num
              ? 'text-indigo-400'
              : 'text-slate-500'}"
          >
            <div
              class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold border-2 transition-all duration-300
            {step > s.num
                ? 'bg-indigo-500 border-indigo-500 text-white shadow-[0_0_15px_rgba(99,102,241,0.4)]'
                : step === s.num
                  ? 'border-indigo-400 text-indigo-400 shadow-[0_0_10px_rgba(99,102,241,0.2)]'
                  : 'border-slate-800 text-slate-600'}"
            >
              {#if step > s.num}
                <CheckCircle size={16} />
              {:else}
                {s.num}
              {/if}
            </div>
            <span class="text-xs font-medium hidden sm:block">{s.label}</span>
          </div>
          {#if i < steps.length - 1}
            <div
              class="flex-1 h-px mx-2 {step > s.num
                ? 'bg-indigo-500'
                : 'bg-slate-800'}"
            ></div>
          {/if}
        {/each}
      </div>
    {/if}

    <!-- Step Content -->
    <div
      class="bg-slate-900/50 backdrop-blur-xl ring-1 ring-white/10 rounded-3xl shadow-[0_0_50px_rgba(0,0,0,0.5)] overflow-hidden relative"
    >
      <!-- ======================== STEP 1: Hardware ======================== -->
      {#if step === 1}
        <div class="p-8" in:fly={{ y: 30, easing: cubicOut }}>
          <h2 class="text-2xl font-bold mb-2 flex items-center gap-3">
            <HardDrive class="text-indigo-400" /> Hardware & Muscle
          </h2>
          <p class="text-slate-400 mb-8 text-sm">
            Verifying the gRPC link to the Rust Agent via Unix Domain Socket.
          </p>

          <div
            class="p-5 rounded-2xl border transition-all duration-500 {muscleStatus?.healthy
              ? 'bg-emerald-500/10 border-emerald-500/30 shadow-[0_0_20px_rgba(16,185,129,0.2)]'
              : 'bg-slate-800/50 border-white/5'} mb-8"
          >
            {#if loading}
              <div class="flex items-center gap-3 text-slate-400">
                <Loader2 size={18} class="animate-spin text-indigo-400" />
                <span>Pinging Muscle Agent...</span>
              </div>
            {:else if muscleStatus?.healthy}
              <div class="space-y-3">
                <div class="flex items-center justify-between">
                  <div
                    class="flex items-center gap-2 text-emerald-400 font-semibold"
                  >
                    <CheckCircle size={18} /> Muscle Online
                  </div>
                  <span
                    class="text-xs text-slate-500 font-mono bg-slate-800 px-2 py-1 rounded"
                    >v{muscleStatus.version}</span
                  >
                </div>
                <div class="flex gap-6 text-xs text-slate-400">
                  <span>CPU: {muscleStatus.cpu}</span>
                  <span>RAM: {muscleStatus.ram_mb}MB</span>
                  <span>UDS: /var/run/kari/agent.sock</span>
                </div>
              </div>
            {:else}
              <div class="flex items-center gap-2 text-rose-400">
                <AlertCircle size={18} />
                {muscleStatus?.error || "Muscle connection failed"}
              </div>
            {/if}
          </div>

          <div class="flex gap-3">
            <button
              on:click={testMuscle}
              disabled={loading}
              class="flex-1 bg-slate-800 hover:bg-slate-700 py-3 rounded-xl font-medium transition-all text-sm"
            >
              Retry Test
            </button>
            <button
              disabled={!muscleStatus?.healthy || loading}
              on:click={() => (step = 2)}
              class="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 py-3 rounded-xl font-bold transition-all"
            >
              Continue →
            </button>
          </div>
        </div>

        <!-- ======================== STEP 2: Database ======================== -->
      {:else if step === 2}
        <div class="p-8" in:fly={{ y: 30, easing: cubicOut }}>
          <h2 class="text-2xl font-bold mb-2 flex items-center gap-3">
            <Database class="text-indigo-400" /> Persistence & Networking
          </h2>
          <p class="text-slate-400 mb-8 text-sm">
            Configure and verify your PostgreSQL connection.
          </p>

          <div class="space-y-5 mb-6">
            <div>
              <label
                for="dbUrl"
                class="block text-xs text-slate-500 mb-2 font-bold uppercase tracking-wider"
                >Database Connection</label
              >
              <input
                id="dbUrl"
                type="text"
                bind:value={dbUrl}
                placeholder="postgres://user:pass@host:5432/dbname?sslmode=disable"
                class="w-full bg-slate-950 border border-slate-800 rounded-xl p-3.5 font-mono text-sm
                     focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all"
              />
            </div>
            <div>
              <label
                for="appDomain"
                class="block text-xs text-slate-500 mb-2 font-bold uppercase tracking-wider"
                >Primary Domain</label
              >
              <input
                id="appDomain"
                type="text"
                bind:value={appDomain}
                placeholder="panel.yourdomain.com"
                class="w-full bg-slate-950 border border-slate-800 rounded-xl p-3.5
                     focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all"
              />
              {#if appDomain && !domainValid}
                <p class="text-rose-400 text-xs mt-1">
                  Must be a valid domain (e.g., panel.example.com)
                </p>
              {/if}
            </div>
          </div>

          <!-- DB Test Result -->
          {#if dbStatus}
            <div
              class="p-5 rounded-2xl border mb-6 transition-all duration-500 {dbStatus.healthy
                ? 'bg-emerald-500/10 border-emerald-500/30 shadow-[0_0_20px_rgba(16,185,129,0.2)]'
                : 'bg-rose-500/10 border-rose-500/30 shadow-[0_0_20px_rgba(244,63,94,0.2)]'}"
              transition:slide
            >
              {#if dbStatus.healthy}
                <div
                  class="flex items-center gap-2 text-emerald-400 font-semibold"
                >
                  <CheckCircle size={16} /> Database reachable at {dbStatus.host}:{dbStatus.port}
                </div>
              {:else}
                <div class="flex items-center gap-2 text-rose-400">
                  <AlertCircle size={16} />
                  {dbStatus.error}
                </div>
              {/if}
            </div>
          {/if}

          <div class="flex gap-3">
            <button
              on:click={() => (step = 1)}
              class="bg-slate-800 hover:bg-slate-700 px-6 py-3 rounded-xl font-medium transition-all text-sm"
            >
              ← Back
            </button>
            <button
              on:click={testDB}
              disabled={loading || !dbUrl}
              class="flex-1 bg-slate-800 hover:bg-slate-700 disabled:opacity-40 py-3 rounded-xl font-medium transition-all text-sm flex items-center justify-center gap-2"
            >
              {#if loading}<Loader2 size={14} class="animate-spin" />{/if}
              Test Connection
            </button>
            <button
              disabled={!dbStatus?.healthy || !domainValid}
              on:click={() => (step = 3)}
              class="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 py-3 rounded-xl font-bold transition-all"
            >
              Continue →
            </button>
          </div>
        </div>

        <!-- ======================== STEP 3: Security ======================== -->
      {:else if step === 3}
        <div class="p-8" in:fly={{ y: 30, easing: cubicOut }}>
          <h2 class="text-2xl font-bold mb-2 flex items-center gap-3">
            <ShieldCheck class="text-indigo-400" /> Security Configuration
          </h2>
          <p class="text-slate-400 mb-8 text-sm">
            Create your admin account and generate the master encryption key.
          </p>

          <div class="space-y-5 mb-8">
            <div>
              <label
                for="adminEmail"
                class="block text-xs text-slate-500 mb-2 font-bold uppercase tracking-wider"
                >Admin Email</label
              >
              <input
                id="adminEmail"
                type="email"
                bind:value={adminEmail}
                class="w-full bg-slate-950 border border-slate-800 rounded-xl p-3.5
                     focus:ring-2 focus:ring-indigo-500 outline-none transition-all"
              />
              {#if adminEmail && !emailValid}
                <p class="text-rose-400 text-xs mt-1">Invalid email format</p>
              {/if}
            </div>
            <div>
              <label
                for="adminPassword"
                class="block text-xs text-slate-500 mb-2 font-bold uppercase tracking-wider"
                >Master Password (≥12 chars)</label
              >
              <input
                id="adminPassword"
                type="password"
                bind:value={adminPassword}
                class="w-full bg-slate-950 border border-slate-800 rounded-xl p-3.5
                     focus:ring-2 focus:ring-indigo-500 outline-none transition-all"
              />
              {#if adminPassword && !passwordValid}
                <p class="text-rose-400 text-xs mt-1">
                  Must be at least 12 characters
                </p>
              {/if}
            </div>
            <div>
              <label
                for="confirmPassword"
                class="block text-xs text-slate-500 mb-2 font-bold uppercase tracking-wider"
                >Confirm Password</label
              >
              <input
                id="confirmPassword"
                type="password"
                bind:value={confirmPassword}
                class="w-full bg-slate-950 border border-slate-800 rounded-xl p-3.5
                     focus:ring-2 focus:ring-indigo-500 outline-none transition-all"
              />
              {#if confirmPassword && !passwordsMatch}
                <p class="text-rose-400 text-xs mt-1">Passwords do not match</p>
              {/if}
            </div>
          </div>

          <!-- Master Key Generation -->
          <div
            class="border border-dashed border-amber-500/30 rounded-xl p-5 mb-8 bg-amber-500/5"
          >
            <div class="flex items-start gap-3 mb-4">
              <AlertTriangle
                size={18}
                class="text-amber-400 mt-0.5 flex-shrink-0"
              />
              <p class="text-sm text-amber-200">
                Click below to generate your <strong
                  >AES-256 Master Encryption Key</strong
                >. This key protects all secrets (SSH keys, env vars). You will
                receive a 24-word recovery phrase.
              </p>
            </div>
            {#if masterKey}
              <div
                class="bg-slate-950 rounded-lg p-4 font-mono text-sm text-emerald-400 break-words select-all border border-emerald-500/20"
              >
                {masterKey.recovery_phrase}
              </div>
              <p class="text-xs text-slate-500 mt-2">
                {masterKey.word_count} words • AES-256-GCM
              </p>
            {:else}
              <button
                on:click={generateKey}
                disabled={loading}
                class="w-full bg-amber-600 hover:bg-amber-500 disabled:opacity-40 py-3 rounded-xl font-bold transition-all flex items-center justify-center gap-2"
              >
                {#if loading}<Loader2 size={14} class="animate-spin" />{/if}
                <Key size={16} /> Generate Master Key
              </button>
            {/if}
          </div>

          <div class="flex gap-3">
            <button
              on:click={() => (step = 2)}
              class="bg-slate-800 hover:bg-slate-700 px-6 py-3 rounded-xl font-medium transition-all text-sm"
            >
              ← Back
            </button>
            <button
              disabled={!securityReady}
              on:click={() => (step = 4)}
              class="flex-1 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 py-3 rounded-xl font-bold transition-all"
            >
              Review & Finish →
            </button>
          </div>
        </div>

        <!-- ======================== STEP 4: Recovery & Confirm ======================== -->
      {:else if step === 4}
        <div class="p-8" in:fly={{ y: 30, easing: cubicOut }}>
          <h2 class="text-2xl font-bold mb-2 flex items-center gap-3">
            <Key class="text-indigo-400" /> Final Review
          </h2>
          <p class="text-slate-400 mb-8 text-sm">
            Confirm your settings and save your recovery phrase before locking
            down.
          </p>

          <!-- Config Summary -->
          <div class="bg-slate-800/50 rounded-xl p-5 mb-6 space-y-3 text-sm">
            <div class="flex justify-between">
              <span class="text-slate-500">Admin</span>
              <span class="font-mono">{adminEmail}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Domain</span>
              <span class="font-mono">{appDomain}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Database</span>
              <span class="font-mono text-xs"
                >{dbUrl.split("@")[1]?.split("/")[0] || "configured"}</span
              >
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Encryption</span>
              <span class="text-emerald-400 font-medium"
                >AES-256-GCM • 24-word phrase</span
              >
            </div>
          </div>

          <!-- Recovery Phrase (Final Copy) -->
          <div
            class="border border-rose-500/30 rounded-xl p-5 mb-8 bg-rose-500/5"
          >
            <div class="flex items-center justify-between mb-3">
              <span
                class="text-xs font-bold uppercase tracking-wider text-rose-400"
                >⚠ Save Your Recovery Phrase</span
              >
              <button
                on:click={copyPhrase}
                class="text-xs flex items-center gap-1 text-slate-400 hover:text-white transition-colors"
              >
                <Copy size={12} />
                {copied ? "Copied!" : "Copy"}
              </button>
            </div>
            <div
              class="bg-slate-950 rounded-lg p-4 font-mono text-sm text-emerald-400 break-words select-all border border-slate-800"
            >
              {masterKey?.recovery_phrase}
            </div>
            <p class="text-xs text-rose-300 mt-3 font-medium">
              This phrase will NEVER be shown again after lockdown. It is the
              ONLY way to recover encrypted secrets if your server is lost.
            </p>
          </div>

          <div class="flex gap-3">
            <button
              on:click={() => (step = 3)}
              class="bg-slate-800 hover:bg-slate-700 px-6 py-3 rounded-xl font-medium transition-all text-sm"
            >
              ← Back
            </button>
            <button
              on:click={finalize}
              disabled={loading}
              class="flex-1 bg-gradient-to-r from-indigo-600 to-violet-600 hover:from-indigo-500 hover:to-violet-500
                   disabled:opacity-40 py-4 rounded-xl font-bold shadow-lg shadow-indigo-500/20 transition-all
                   flex items-center justify-center gap-2"
            >
              {#if loading}
                <Loader2 size={16} class="animate-spin" />
                Locking Down...
              {:else}
                <Lock size={16} /> Finalize & Lock System
              {/if}
            </button>
          </div>
        </div>

        <!-- ======================== STEP 5: Lockdown Complete ======================== -->
      {:else if step === 5}
        <div class="p-10 text-center" in:fade>
          <div
            class="w-20 h-20 bg-emerald-500/10 text-emerald-500 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-emerald-500/20"
          >
            <Zap size={36} />
          </div>
          <h2 class="text-3xl font-bold mb-3 text-emerald-400">
            System Locked Down
          </h2>
          <p class="text-slate-400 mb-8 max-w-md mx-auto">
            Your Karı Panel is now in <strong>Production Mode</strong>. The
            setup wizard has been permanently disabled.
          </p>

          <div class="bg-slate-800/50 rounded-xl p-5 mb-8 text-sm space-y-2">
            <div
              class="flex items-center justify-center gap-2 text-emerald-400"
            >
              <CheckCircle size={14} /> Configuration saved to .env.production
            </div>
            <div
              class="flex items-center justify-center gap-2 text-emerald-400"
            >
              <CheckCircle size={14} /> setup.lock created (read-only)
            </div>
            <div class="flex items-center justify-center gap-2 text-amber-400">
              <Loader2 size={14} class="animate-spin" /> Brain restarting in Production
              Mode...
            </div>
          </div>

          <p class="text-xs text-slate-600 mb-8">
            The page will automatically redirect when the server is back online.
          </p>

          <a
            href="/login"
            class="inline-block w-full bg-indigo-600 hover:bg-indigo-500 py-4 rounded-xl font-bold transition-all"
          >
            Go to Secure Login
          </a>
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <p class="text-center text-xs text-slate-600 mt-8">
      Karı Orchestration Engine • Zero-Trust • SOLID • SLA-Compliant
    </p>
  </div>
</main>
