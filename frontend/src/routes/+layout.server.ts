import type { LayoutServerLoad } from './$types';

// 🛡️ SLA: The Server-to-Client Handshake
// This root layout load function runs on the server. Whatever is returned here 
// is serialized by SvelteKit and injected directly into the $page.data store 
// for every route in the application.
export const load: LayoutServerLoad = async ({ locals }) => {
	return {
		// 🛡️ Zero-Trust Data Exposure:
		// We only expose the exact fields needed for UI rendering and RBAC 
		// (Role-Based Access Control). Because we already stripped out sensitive 
		// data (like the JWT itself or password hashes) back in the hook, 
		// this object is 100% safe to serialize to the browser.
		user: locals.user ? {
			id: locals.user.id,
			email: locals.user.email,
			rank: locals.user.rank,
			permissions: locals.user.permissions
		} : null
	};
};
