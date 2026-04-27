<script lang="ts">
    import type { PageData } from './$types';
    import { API } from '$lib/api/client';

    export let data: PageData;
    
    // Reactive data bindings
    $: roles = data.roles;
    $: permissionMatrix = data.permissionMatrix;
    // We keep a local draft of the mappings so we can mutate it before saving
    let draftMappings = { ...data.roleMappings };

    // UI State
    let selectedRoleId: string | null = roles.length > 0 ? roles[0].id : null;
    let isSaving = false;
    let saveMessage: { type: 'success' | 'error', text: string } | null = null;

    // Derived reactive state for the currently selected role
    $: selectedRole = roles.find(r => r.id === selectedRoleId);
    $: currentRolePerms = new Set(draftMappings[selectedRoleId || ''] || []);

    // ==============================================================================
    // Actions
    // ==============================================================================

    function selectRole(id: string) {
        selectedRoleId = id;
        saveMessage = null;
    }

    function togglePermission(permissionId: string) {
        if (!selectedRoleId || selectedRole?.is_system) return; // Prevent editing system roles

        const updatedPerms = new Set(currentRolePerms);
        if (updatedPerms.has(permissionId)) {
            updatedPerms.delete(permissionId);
        } else {
            updatedPerms.add(permissionId);
        }
        
        // Update the draft state
        draftMappings[selectedRoleId] = Array.from(updatedPerms);
        draftMappings = { ...draftMappings }; // Trigger Svelte reactivity
    }

    async function saveRolePermissions() {
        if (!selectedRoleId || isSaving || selectedRole?.is_system) return;

        isSaving = true;
        saveMessage = null;

        try {
            // Push the array of assigned Permission IDs to the Go API
            await API.put(`/api/v1/roles/${selectedRoleId}/permissions`, {
                permission_ids: draftMappings[selectedRoleId]
            });
            
            saveMessage = { type: 'success', text: 'Role permissions successfully updated.' };
            setTimeout(() => { saveMessage = null; }, 3000);
        } catch (error: any) {
            saveMessage = { type: 'error', text: error.message || 'Failed to update permissions.' };
        } finally {
            isSaving = false;
        }
    }
</script>

<svelte:head>
    <title>Roles & Permissions - Karı Control Panel</title>
</svelte:head>

<div class="mb-8">
    <h2 class="text-2xl font-sans font-bold text-kari-text">Access Control</h2>
    <p class="mt-1 text-sm text-kari-warm-gray">Define granular RBAC policies for your team. System roles cannot be modified.</p>
</div>

<div class="flex flex-col lg:flex-row gap-8 h-[700px]">
    
    <div class="w-full lg:w-1/3 flex flex-col gap-4">
        <div class="card bg-white flex-1 overflow-hidden flex flex-col">
            <div class="px-4 py-3 border-b border-kari-warm-gray/20 bg-gray-50/50 flex justify-between items-center">
                <h3 class="font-sans font-semibold text-kari-text">Roles</h3>
                <button class="text-xs font-medium text-kari-teal hover:text-[#158C85]">+ New Role</button>
            </div>
            <ul class="flex-1 overflow-y-auto divide-y divide-kari-warm-gray/10">
                {#each roles as role}
                    <li>
                        <button 
                            on:click={() => selectRole(role.id)}
                            class="w-full text-left px-4 py-4 transition-colors hover:bg-kari-light-gray/50 {selectedRoleId === role.id ? 'bg-kari-teal/5 border-l-4 border-kari-teal' : 'border-l-4 border-transparent'}"
                        >
                            <div class="flex items-center justify-between">
                                <span class="font-sans font-medium {selectedRoleId === role.id ? 'text-kari-teal' : 'text-kari-text'}">
                                    {role.name}
                                </span>
                                {#if role.is_system}
                                    <span class="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-kari-warm-gray/10 text-kari-warm-gray">
                                        System
                                    </span>
                                {/if}
                            </div>
                            <p class="text-xs text-kari-warm-gray mt-1 line-clamp-2">{role.description}</p>
                        </button>
                    </li>
                {/each}
            </ul>
        </div>
    </div>

    <div class="w-full lg:w-2/3 flex flex-col">
        {#if selectedRole}
            <div class="card bg-white flex-1 flex flex-col overflow-hidden">
                
                <div class="px-6 py-5 border-b border-kari-warm-gray/20 bg-gray-50/50 shrink-0 flex justify-between items-center">
                    <div>
                        <h3 class="text-lg font-sans font-semibold text-kari-text">{selectedRole.name} Permissions</h3>
                        {#if selectedRole.is_system}
                            <p class="mt-1 text-xs text-yellow-600 font-medium">This is a protected system role and cannot be modified.</p>
                        {:else}
                            <p class="mt-1 text-xs text-kari-warm-gray">Toggle permissions below to customize access.</p>
                        {/if}
                    </div>
                </div>

                <div class="flex-1 overflow-y-auto p-6 bg-kari-light-gray/10">
                    <div class="space-y-8">
                        {#each Object.entries(permissionMatrix) as [resource, permissions]}
                            <div class="bg-white border border-kari-warm-gray/20 rounded-lg overflow-hidden shadow-sm">
                                <div class="px-4 py-3 bg-gray-50/50 border-b border-kari-warm-gray/20">
                                    <h4 class="font-sans font-semibold text-sm text-kari-text capitalize">{resource.replace('_', ' ')}</h4>
                                </div>
                                <div class="divide-y divide-kari-warm-gray/10">
                                    {#each permissions as perm}
                                        <div class="px-4 py-3 flex items-center justify-between hover:bg-kari-light-gray/20 transition-colors">
                                            <div>
                                                <p class="text-sm font-medium text-kari-text capitalize">{perm.action}</p>
                                                <p class="text-xs text-kari-warm-gray">{perm.description}</p>
                                            </div>
                                            
                                            <button 
                                                type="button" 
                                                role="switch" 
                                                aria-checked={currentRolePerms.has(perm.id)}
                                                disabled={selectedRole.is_system}
                                                on:click={() => togglePermission(perm.id)}
                                                class="
                                                    relative inline-flex flex-shrink-0 h-6 w-11 border-2 border-transparent rounded-full cursor-pointer transition-colors ease-in-out duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-teal disabled:opacity-50 disabled:cursor-not-allowed
                                                    {currentRolePerms.has(perm.id) ? 'bg-kari-teal' : 'bg-kari-warm-gray/30'}
                                                "
                                            >
                                                <span 
                                                    class="
                                                        pointer-events-none inline-block h-5 w-5 rounded-full bg-white shadow transform ring-0 transition ease-in-out duration-200
                                                        {currentRolePerms.has(perm.id) ? 'translate-x-5' : 'translate-x-0'}
                                                    "
                                                ></span>
                                            </button>
                                        </div>
                                    {/each}
                                </div>
                            </div>
                        {/each}
                    </div>
                </div>

                {#if !selectedRole.is_system}
                    <div class="px-6 py-4 border-t border-kari-warm-gray/20 bg-gray-50/50 shrink-0 flex items-center justify-between">
                        <div class="flex-1 mr-4">
                            {#if saveMessage}
                                <p class={`text-sm font-medium ${saveMessage.type === 'error' ? 'text-red-600' : 'text-kari-teal'}`}>
                                    {saveMessage.text}
                                </p>
                            {/if}
                        </div>
                        <button 
                            on:click={saveRolePermissions}
                            disabled={isSaving}
                            class="inline-flex items-center px-4 py-2 border border-transparent rounded shadow-sm text-sm font-sans font-medium text-white bg-kari-text hover:bg-black focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-kari-text transition-colors disabled:opacity-70"
                        >
                            {#if isSaving}
                                Saving...
                            {:else}
                                Save Policies
                            {/if}
                        </button>
                    </div>
                {/if}

            </div>
        {:else}
            <div class="card bg-white flex-1 flex flex-col items-center justify-center text-center p-8">
                <svg class="h-12 w-12 text-kari-warm-gray/40 mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
                <h3 class="text-lg font-sans font-medium text-kari-text">Select a Role</h3>
                <p class="mt-1 text-sm text-kari-warm-gray">Choose a role from the left pane to configure its access policies.</p>
            </div>
        {/if}
    </div>
</div>
