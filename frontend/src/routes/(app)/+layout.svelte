<script lang="ts">
    import { page } from '$app/stores';
    import { fade } from 'svelte/transition';
    import type { LayoutData } from './$types';
    import { 
        LayoutDashboard, 
        Boxes, 
        Globe, 
        Bell, 
        Settings, 
        LogOut,
        User as UserIcon
    } from 'lucide-svelte';

    // Data securely provided by +layout.server.ts
    export let data: LayoutData;
    $: user = data.user;

    // Standardized navigation map using Lucide components
    const navItems = [
        { name: 'Dashboard', path: '/dashboard', icon: LayoutDashboard },
        { name: 'Applications', path: '/applications', icon: Boxes },
        { name: 'Domains', path: '/domains', icon: Globe },
        { name: 'Action Center', path: '/action-center', icon: Bell },
        { name: 'Settings', path: '/settings', icon: Settings }
    ];

    // 🛡️ Logic: Precision active state detection
    $: currentPath = $page.url.pathname;
    const isActive = (path: string) => {
        if (path === '/dashboard' && currentPath === '/') return true;
        return currentPath.startsWith(path);
    };
</script>

<div class="flex h-screen w-full bg-kari-light-gray font-body antialiased text-kari-text">
    
    <aside class="w-64 flex flex-col bg-white border-r border-kari-warm-gray/20 shadow-sm shrink-0">
        <div class="h-16 flex items-center px-6 border-b border-kari-warm-gray/10">
            <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded bg-kari-teal flex items-center justify-center text-white font-sans font-bold text-lg shadow-sm">
                    K
                </div>
                <span class="font-sans font-semibold tracking-tight text-xl text-kari-text">Karı</span>
            </div>
        </div>

        <nav class="flex-1 px-4 py-6 space-y-1 overflow-y-auto">
            {#each navItems as item}
                <a 
                    href={item.path}
                    class="flex items-center gap-3 px-3 py-2.5 rounded-md transition-all duration-200 font-medium group
                           {isActive(item.path) 
                               ? 'bg-kari-teal/10 text-kari-teal' 
                               : 'text-kari-warm-gray hover:bg-gray-50 hover:text-kari-text'}"
                >
                    <svelte:component 
                        this={item.icon} 
                        size={18} 
                        strokeWidth={isActive(item.path) ? 2.5 : 2}
                    />
                    <span class="text-sm">{item.name}</span>
                </a>
            {/each}
        </nav>

        {#if user}
            <div class="p-4 border-t border-kari-warm-gray/20 bg-gray-50/30">
                <div class="flex items-center gap-3 mb-4">
                    <div class="w-10 h-10 rounded-full bg-white border border-kari-warm-gray/20 flex items-center justify-center text-kari-teal shadow-sm">
                        <UserIcon size={20} />
                    </div>
                    <div class="flex flex-col overflow-hidden">
                        <span class="text-xs font-bold text-kari-text truncate">{user.id.split('-')[0]}</span>
                        <div class="flex items-center gap-1">
                            <span class="w-1.5 h-1.5 rounded-full bg-kari-teal animate-pulse"></span>
                            <span class="text-[10px] uppercase font-bold tracking-wider text-kari-warm-gray">
                                {user.role || 'Operator'}
                            </span>
                        </div>
                    </div>
                </div>
                
                <form method="POST" action="/logout">
                    <button 
                        type="submit" 
                        class="w-full flex items-center gap-2 px-3 py-2 rounded-md text-xs font-bold text-kari-warm-gray hover:text-red-500 hover:bg-red-50 transition-all group"
                    >
                        <LogOut size={14} class="group-hover:translate-x-0.5 transition-transform" />
                        Sign out
                    </button>
                </form>
            </div>
        {/if}
    </aside>

    <main class="flex-1 flex flex-col min-w-0 overflow-hidden">
        <header class="h-16 flex items-center justify-between px-8 border-b border-kari-warm-gray/10 bg-white/80 backdrop-blur-md shrink-0">
            <h1 class="font-sans font-bold text-kari-text text-base uppercase tracking-widest">
                {currentPath === '/' ? 'Dashboard' : currentPath.split('/')[1].replace('-', ' ')}
            </h1>
            
            <div class="flex items-center gap-4">
                <div class="h-2 w-2 rounded-full bg-emerald-500"></div>
                <span class="text-[10px] font-bold text-kari-warm-gray uppercase tracking-widest">Muscle Link: Active</span>
            </div>
        </header>

        <div class="flex-1 overflow-y-auto p-8 bg-kari-light-gray/30">
            <div class="max-w-6xl mx-auto">
                {#key currentPath}
                    <div in:fade={{ duration: 150 }}>
                        <slot />
                    </div>
                {/key}
            </div>
        </div>
    </main>

</div>
