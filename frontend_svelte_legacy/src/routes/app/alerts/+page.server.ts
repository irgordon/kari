import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { env } from '$env/dynamic/private';

export const load: PageServerLoad = async ({ locals, url, fetch }) => {
	// 🛡️ 1. Zero-Trust Gateway
	// Check if the user was even authenticated by hooks.server.ts
	if (!locals.user) {
		throw redirect(303, '/auth/login');
	}

	// 🛡️ 2. Permission Guard (SOLID - SRP)
	// We check for the specific 'alerts:read' permission. 
	// This maps directly to our Go Brain's RBAC middleware requirements.
	if (!locals.user.permissions.includes('alerts:read')) {
		throw error(403, 'Forbidden: You do not have permission to view system alerts');
	}

	// 🛡️ 3. URL Parameter Extraction & Sanitization
	// We pass these directly to the AuditRepository through the Go API.
	const severity = url.searchParams.get('severity') || '';
	const isResolved = url.searchParams.get('resolved') === 'true';
	const limit = Math.min(parseInt(url.searchParams.get('limit') || '10'), 50);
	const offset = parseInt(url.searchParams.get('offset') || '0');

	try {
		// 🛡️ 4. Tenant-Aware Data Fetching
		// Note how we pass the user's Rank and ID. 
		// The Go Brain uses this to filter the GIN-indexed JSONB logs.
		const queryParams = new URLSearchParams({
			severity,
			is_resolved: isResolved.toString(),
			limit: limit.toString(),
			offset: offset.toString()
		});

		const response = await fetch(`${env.KARI_API_URL}/alerts?${queryParams}`);

		if (!response.ok) {
			if (response.status === 401) throw redirect(303, '/auth/login');
			throw error(response.status, 'Failed to sync with Karı Brain');
		}

		const data = await response.json();

		// 🛡️ 5. Data Scrubbing (SLA Compliance)
		// We only return what the UI needs, stripping internal Go-specific fields.
		return {
			alerts: data.alerts,
			totalCount: data.total_count,
			filters: {
				severity,
				isResolved,
				limit,
				offset
			}
		};
	} catch (err) {
		console.error('Action Center Load Error:', err);
		throw error(500, 'Infrastructure synchronization failure');
	}
};
