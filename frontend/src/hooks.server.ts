import type { Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import * as jose from 'jose'; // 🛡️ High-performance crypto standard

interface KariJwtPayload extends jose.JWTPayload {
	sub: string;
	email: string;
	rank: string;
	permissions: string[];
}

export const handle: Handle = async ({ event, resolve }) => {
	// 1. Static Asset Bypass
	if (event.url.pathname.startsWith('/_app') || event.url.pathname.includes('.')) {
		return await resolve(event);
	}

	const accessToken = event.cookies.get('kari_access_token');
	event.locals.user = null;

	// 2. 🛡️ Stateless Cryptographic Verification
	if (accessToken) {
		try {
			// Convert our secret into the format jose expects
			const secret = new TextEncoder().encode(env.JWT_SECRET);
			
			// jwtVerify automatically checks the signature AND the 'exp' (expiration) date!
			const { payload } = await jose.jwtVerify(accessToken, secret) as { payload: KariJwtPayload };

			// 🛡️ Zero Latency: We immediately populate locals without a network request
			event.locals.user = {
				id: payload.sub,
				email: payload.email,
				rank: payload.rank,
				permissions: payload.permissions
			};

		} catch (error) {
			// 🛡️ Security: If the token is expired, tampered with, or invalid, we drop it silently.
			// Next steps would normally include the Silent Refresh logic here.
			console.error('🚨 [Auth Hook] Token verification failed:', (error as Error).message);
			event.cookies.delete('kari_access_token', { path: '/' });
		}
	}

	// 3. Zero-Trust Route Protection
	const isProtectedRoute = event.url.pathname.startsWith('/dashboard') || event.url.pathname.startsWith('/app');

	if (isProtectedRoute && !event.locals.user) {
		const redirectTo = encodeURIComponent(event.url.pathname + event.url.search);
		return new Response(null, {
			status: 303,
			headers: { Location: `/login?redirectTo=${redirectTo}` }
		});
	}

	return await resolve(event);
};
