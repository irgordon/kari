<script lang="ts">
    import { enhance } from '$app/forms';
    import { page } from '$app/stores';
    import type { ActionData } from './$types';

    // Captures error messages returned by `fail()` in +page.server.ts
    export let form: ActionData;

    let isLoading = false;
    
    // Check if the user was kicked out due to an expired session
    $: sessionExpired = $page.url.searchParams.get('session') === 'expired';
</script>

<svelte:head>
    <title>Log in - Karı Control Panel</title>
</svelte:head>

<div class="min-h-screen bg-kari-light-gray flex flex-col justify-center py-12 sm:px-6 lg:px-8 font-body antialiased">
    
    <div class="sm:mx-auto sm:w-full sm:max-w-md">
        <div class="flex justify-center">
            <div class="w-12 h-12 rounded bg-kari-teal flex items-center justify-center text-white font-sans font-bold text-2xl shadow-sm">
                K
            </div>
        </div>
        <h2 class="mt-6 text-center text-3xl font-sans font-extrabold text-kari-text tracking-tight">
            Sign in to Karı
        </h2>
        <p class="mt-2 text-center text-sm text-kari-warm-gray">
            Platform-Agnostic Orchestration Engine
        </p>
    </div>

    <div class="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
        <div class="bg-white py-8 px-4 shadow sm:rounded-lg sm:px-10 border border-kari-warm-gray/20">
            
            {#if form?.message}
                <div class="mb-4 bg-red-50 border-l-4 border-red-500 p-4 rounded-md">
                    <div class="flex">
                        <div class="flex-shrink-0">
                            <svg class="h-5 w-5 text-red-500" viewBox="0 0 20 20" fill="currentColor">
                                <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd" />
                            </svg>
                        </div>
                        <div class="ml-3">
                            <p class="text-sm text-red-700 font-medium">{form.message}</p>
                        </div>
                    </div>
                </div>
            {/if}

            {#if sessionExpired && !form?.message}
                <div class="mb-4 bg-yellow-50 border-l-4 border-yellow-400 p-4 rounded-md">
                    <div class="flex">
                        <div class="flex-shrink-0">
                            <svg class="h-5 w-5 text-yellow-400" viewBox="0 0 20 20" fill="currentColor">
                                <path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
                            </svg>
                        </div>
                        <div class="ml-3">
                            <p class="text-sm text-yellow-700 font-medium">Your session expired. Please log in again.</p>
                        </div>
                    </div>
                </div>
            {/if}

            <form class="space-y-6" method="POST" use:enhance={() => {
                isLoading = true;
                return async ({ update }) => {
                    await update();
                    isLoading = false;
                };
            }}>
                <div>
                    <label for="email" class="block text-sm font-medium text-kari-text">
                        Email address
                    </label>
                    <div class="mt-1">
                        <input 
                            id="email" 
                            name="email" 
                            type="email" 
                            autocomplete="email" 
                            required 
                            value={form?.email ?? ''}
                            class="appearance-none block w-full px-3 py-2 border border-kari-warm-gray/30 rounded-md shadow-sm placeholder-kari-warm-gray/70 focus:outline-none focus:ring-kari-teal focus:border-kari-teal sm:text-sm text-kari-text" 
                            placeholder="admin@example.com"
                        >
                    </div>
                </div>

                <div>
                    <label for="password" class="block text-sm font-medium text-kari-text">
                        Password
                    </label>
                    <div class="mt-1">
                        <input 
                            id="password" 
                            name="password" 
                            type="password" 
                            autocomplete="current-password" 
                            required 
                            class="appearance-none block w-full px-3 py-2 border border-kari-warm-gray/30 rounded-md shadow-sm placeholder-kari-warm-gray/70 focus:outline-none focus:ring-kari-teal focus:border-kari-teal sm:text-sm text-kari-text"
                        >
                    </div>
                </div>

                <div>
                    <button 
                        type="submit" 
                        disabled={isLoading}
                        class="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-sans font-medium text-white bg-kari-teal hover:bg-[#158C85] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-teal transition-colors duration-200 disabled:opacity-70 disabled:cursor-not-allowed"
                    >
                        {#if isLoading}
                            <svg class="animate-spin -ml-1 mr-3 h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                            Authenticating...
                        {:else}
                            Sign in
                        {/if}
                    </button>
                </div>
            </form>
            
        </div>
    </div>
</div>
