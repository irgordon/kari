// frontend/src/routes/(app)/settings/roles/+page.ts
import type { PageLoad } from './$types';

// ==============================================================================
// 1. SLA Type Definitions
// ==============================================================================

export interface Role {
    id: string;
    name: string;
    description: string;
    is_system: boolean; // Protects the "Super Admin" role from being deleted
}

export interface Permission {
    id: string;
    resource: string; // e.g., "applications", "domains", "audit_logs"
    action: string;   // e.g., "read", "write", "delete", "deploy"
    description: string;
}

// Maps a Role ID to an array of Permission IDs
export type RolePermissionMap = Record<string, string[]>;

export const load: PageLoad = async ({ fetch }) => {
    const apiUrl = import.meta.env.VITE_API_URL || 'http://localhost:8080';

    try {
        // Fetch the Roles, the Master Permissions List, and the Current Mappings
        const [rolesRes, permsRes, mappingsRes] = await Promise.all([
            fetch(`${apiUrl}/api/v1/roles`),
            fetch(`${apiUrl}/api/v1/permissions`),
            fetch(`${apiUrl}/api/v1/roles/mappings`)
        ]);

        if (!rolesRes.ok || !permsRes.ok || !mappingsRes.ok) {
            throw new Error('Failed to load RBAC data from the Go API.');
        }

        const roles: Role[] = await rolesRes.json();
        const permissions: Permission[] = await permsRes.json();
        const roleMappings: RolePermissionMap = await mappingsRes.json();

        // Structure the flat permissions array into a Matrix grouped by Resource 
        // to make the UI rendering extremely efficient.
        const permissionMatrix: Record<string, Permission[]> = {};
        for (const perm of permissions) {
            if (!permissionMatrix[perm.resource]) {
                permissionMatrix[perm.resource] = [];
            }
            permissionMatrix[perm.resource].push(perm);
        }

        return {
            roles,
            permissionMatrix,
            roleMappings
        };
    } catch (error) {
        console.error('[RBAC Load] Error:', error);
        return {
            roles: [],
            permissionMatrix: {},
            roleMappings: {}
        };
    }
};
