<script lang="ts">
    import type { PageData } from './$types';
    
    // SvelteKit automatically populates this with the return value of +page.ts
    export let data: PageData;
    
    // Reactive statement to bind the data
    $: apps = data.applications;

    // SLA Formatting Helper: Decouples raw data format from UI presentation
    const formatAppType = (type: string) => {
        const types: Record<string, string> = {
            'nodejs': 'Node.js',
            'python': 'Python',
            'php': 'PHP',
            'ruby': 'Ruby',
            'static': 'Static Site'
        };
        return types[type] || type;
    };

    const extractRepoName = (url: string) => {
        // Converts "https://github.com/user/repo.git" to "user/repo"
        return url.replace(/^https?:\/\/(www\.)?github\.com\//, '').replace(/\.git$/, '');
    };
</script>

<svelte:head>
    <title>Applications - Karı Control Panel</title>
</svelte:head>

<div class="mb-8 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
    <div>
        <h2 class="text-2xl font-sans font-bold text-kari-text">Deployed Applications</h2>
        <p class="mt-1 text-sm text-kari-warm-gray">Manage your GitOps deployments and environment variables.</p>
    </div>
    <div class="flex-shrink-0">
        <a 
            href="/applications/new" 
            class="inline-flex items-center justify-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-sans font-medium text-white bg-kari-teal hover:bg-[#158C85] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-teal transition-colors"
        >
            <svg xmlns="http://www.w3.org/2000/svg" class="-ml-1 mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
            </svg>
            New Application
        </a>
    </div>
</div>

{#if apps.length === 0}
    <div class="card p-12 flex flex-col items-center justify-center text-center border-dashed border-2">
        <div class="h-16 w-16 bg-kari-light-gray rounded-full flex items-center justify-center mb-4">
            <svg class="h-8 w-8 text-kari-warm-gray" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 002-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
            </svg>
        </div>
        <h3 class="text-lg font-sans font-medium text-kari-text">No applications found</h3>
        <p class="mt-2 text-sm text-kari-warm-gray max-w-sm">You haven't deployed any applications yet. Connect a Git repository to get started with zero-downtime deployments.</p>
        <div class="mt-6">
            <a href="/applications/new" class="text-kari-teal hover:text-[#158C85] font-medium text-sm flex items-center gap-1">
                Deploy your first app
                <span aria-hidden="true">&rarr;</span>
            </a>
        </div>
    </div>
{:else}
    <div class="card overflow-x-auto">
        <table class="min-w-full divide-y divide-kari-warm-gray/20">
            <thead class="bg-gray-50/50">
                <tr>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-kari-warm-gray uppercase tracking-wider">Repository</th>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-kari-warm-gray uppercase tracking-wider">Environment</th>
                    <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-kari-warm-gray uppercase tracking-wider">Status</th>
                    <th scope="col" class="relative px-6 py-3"><span class="sr-only">Actions</span></th>
                </tr>
            </thead>
            <tbody class="bg-white divide-y divide-kari-warm-gray/10">
                {#each apps as app}
                    <tr class="hover:bg-kari-light-gray/50 transition-colors">
                        <td class="px-6 py-4 whitespace-nowrap">
                            <div class="flex items-center">
                                <div class="flex-shrink-0 h-10 w-10 flex items-center justify-center rounded bg-kari-teal/10 text-kari-teal font-sans font-bold">
                                    {app.app_type.charAt(0).toUpperCase()}
                                </div>
                                <div class="ml-4">
                                    <div class="text-sm font-medium text-kari-text">
                                        {extractRepoName(app.repo_url)}
                                    </div>
                                    <div class="text-xs text-kari-warm-gray flex items-center gap-1 mt-0.5">
                                        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7v8a2 2 0 002 2h6M8 7V5a2 2 0 012-2h4.586a1 1 0 01.707.293l4.414 4.414a1 1 0 01.293.707V15a2 2 0 01-2 2h-2M8 7H6a2 2 0 00-2 2v10a2 2 0 002 2h8a2 2 0 002-2v-2"></path></svg>
                                        {app.branch}
                                    </div>
                                </div>
                            </div>
                        </td>

                        <td class="px-6 py-4 whitespace-nowrap">
                            <div class="text-sm text-kari-text">{formatAppType(app.app_type)}</div>
                            <div class="text-xs text-kari-warm-gray mt-0.5">ID: {app.id.split('-')[0]}</div>
                        </td>

                        <td class="px-6 py-4 whitespace-nowrap">
                            <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                <span class="w-1.5 h-1.5 mr-1.5 bg-green-500 rounded-full"></span>
                                Running
                            </span>
                        </td>

                        <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                            <a href={`/applications/${app.id}`} class="text-kari-teal hover:text-[#158C85] transition-colors">
                                Manage<span class="sr-only">, {extractRepoName(app.repo_url)}</span>
                            </a>
                        </td>
                    </tr>
                {/each}
            </tbody>
        </table>
    </div>
{/if}
