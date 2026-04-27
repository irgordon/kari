import type { LayoutServerLoad } from './$types';

// 🛡️ SLA: The Server-to-Client Handshake
// This root layout load function runs strictly on the Node.js server. 
// Whatever is returned here is serialized by SvelteKit and injected directly 
// into the reactive `$page.data` store for every single route in the application.
export const load: LayoutServerLoad = async ({ locals }) => {
	return {
		// 🛡️ Zero-Trust Data Exposure:
		// We explicitly map the fields rather than passing the whole `locals.user` object.
		// This guarantees that even if a developer accidentally attaches sensitive 
		// data (like a raw JWT string or a password hash) to `locals.user` in the future, 
		// it will NEVER be serialized and leaked to the browser's HTML payload.
		user: locals.user ? {
			id: locals.user.id,
			email: locals.user.email,
			rank: locals.user.rank,
			permissions: locals.user.permissions
		} : null
	};
};
