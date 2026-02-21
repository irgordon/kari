<script lang="ts">
    import { enhance } from '$app/forms';
    import type { ActionData, PageData } from './$types';
    import Alert from '$components/Alert.svelte'; // Hypothetical Tailwind component

    export let data: PageData;
    export let form: ActionData;

    // Reactively assign the profile from the server load function
    $: profile = data.profile;
    
    let isSubmitting = false;
</script>

<svelte:head>
    <title>System Settings | Karı Orchestration</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6 space-y-8">
    <div>
        <h1 class="text-2xl font-bold text-kari-text">System Governance</h1>
        <p class="text-kari-warm-gray text-sm mt-1">Configure global resource limits and SLA policies.</p>
    </div>

    {#if form?.error}
        <Alert type="error" title="Update Failed">
            {form.error}
            {#if form.conflict}
                <button 
                    class="ml-4 underline font-medium text-red-700 hover:text-red-900"
                    on:click={() => window.location.reload()}
                >
                    Refresh Data
                </button>
            {/if}
        </Alert>
    {/if}

    {#if form?.success}
        <Alert type="success" title="Success">
            System profile updated successfully. The Rust Agent is synchronizing state.
        </Alert>
    {/if}

    <form 
        method="POST" 
        action="?/updateProfile"
        use:enhance={() => {
            isSubmitting = true;
            return async ({ update }) => {
                await update();
                isSubmitting = false;
            };
        }}
        class="bg-white shadow-sm ring-1 ring-gray-900/5 sm:rounded-xl p-6 space-y-6"
    >
        <input type="hidden" name="version" value={profile?.version} />

        <div class="grid grid-cols-1 gap-x-6 gap-y-8 sm:grid-cols-2">
            <div>
                <label for="maxMemory" class="block text-sm font-medium leading-6 text-kari-text">
                    Max Memory Per App (MB)
                </label>
                <div class="mt-2">
                    <input 
                        type="number" 
                        name="maxMemory" 
                        id="maxMemory" 
                        value={profile?.max_memory_per_app_mb}
                        min="128"
                        required
                        class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-kari-teal sm:text-sm sm:leading-6"
                    >
                </div>
            </div>

            <div>
                <label for="maxCpu" class="block text-sm font-medium leading-6 text-kari-text">
                    Max CPU Allocation (%)
                </label>
                <div class="mt-2">
                    <input 
                        type="number" 
                        name="maxCpu" 
                        id="maxCpu" 
                        value={profile?.max_cpu_percent_per_app}
                        min="10"
                        max="100"
                        required
                        class="block w-full rounded-md border-0 py-1.5 text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 focus:ring-2 focus:ring-inset focus:ring-kari-teal sm:text-sm sm:leading-6"
                    >
                </div>
            </div>
        </div>

        <div class="flex items-center justify-end border-t border-gray-900/10 pt-6">
            <button 
                type="submit" 
                disabled={isSubmitting}
                class="rounded-md bg-kari-teal px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-kari-teal-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-kari-teal disabled:opacity-50"
            >
                {isSubmitting ? 'Saving...' : 'Save Configuration'}
            </button>
        </div>
    </form>
</div>
