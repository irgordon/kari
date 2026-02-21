import type { Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';

export const handle: Handle = async ({ event, resolve }) => {
	// 1. 🛡️ Performance: Static Asset Bypass
	// Never waste CPU cycles or network hops verifying auth for public static files.
	if (event.url.pathname.startsWith('/_app') || event.url.pathname.includes('.')) {
		return await resolve(event);
	}

	const accessToken = event.cookies.get('kari_access_token');
	const refreshToken = event.cookies.get('kari_refresh_token');
	
	event.locals.user = null;

	// 2. 🛡️ SLA: Centralized Verification via the Brain
	if (accessToken) {
		try {
			// Use event.fetch to ensure internal Docker routing is respected
			const baseUrl = env.INTERNAL_API_URL || env.KARI_API_URL || 'http://api:8080';
			const response = await event.fetch(`${baseUrl}/api/v1/auth/me`, {
				headers: { Authorization: `Bearer ${accessToken}` }
			});

			if (response.ok) {
				const userData = await response.json();
				event.locals.user = {
					id: userData.id,
					email: userData.email,
					rank: userData.rank,
					permissions: userData.permissions
				};
			} else if (response.status === 401 && refreshToken) {
				// 🛡️ Progression: The token expired, but we have a refresh token!
				// Here is where you would call your /api/v1/auth/refresh endpoint
				// (as we established in the earlier JWT iteration) to seamlessly 
				// issue a new access token without kicking the user to the login screen.
				
				// For now, if it fails, we clear the dead token:
				event.cookies.delete('kari_access_token', { path: '/' });
			}
		} catch (err) {
			// 🛡️ Privacy & Compliance: Log technically, fail securely
			console.error('🚨 [Auth Hook] Brain Communication Failure:', err);
		}
	}

	// 3. 🛡️ Zero-Trust Route Protection
	// Define the base paths that require authentication (e.g., /dashboard, /app)
	const isProtectedRoute = event.url.pathname.startsWith('/dashboard') || event.url.pathname.startsWith('/app');

	if (isProtectedRoute && !event.locals.user) {
		// Preserve the intended destination so they land smoothly after logging in
		const redirectTo = encodeURIComponent(event.url.pathname + event.url.search);
		return new Response(null, {
			status: 303,
			headers: { Location: `/login?redirectTo=${redirectTo}` }
		});
	}

	return await resolve(event);
};
