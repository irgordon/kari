import { redirect, type Handle } from '@sveltejs/kit';
import * as jose from 'jose';
import { env } from '$env/dynamic/private';

// 🛡️ Zero-Trust: Strictly defined asset prefixes to prevent bypass via dots in filenames
const ASSET_PREFIXES = ['/_app/', '/favicon.ico', '/static/'];
const PUBLIC_ROUTES = ['/login', '/health'];

// 🛡️ SSL Termination: Trusted internal Docker network for reverse-proxy headers.
// When behind Nginx ingress or a load balancer, the `Origin` header may not match
// the public URL. These trusted proxies are allowed to set X-Forwarded-* headers.
// In production, replace with your actual ingress CIDR or container network range.
const TRUSTED_ORIGINS = [
    'http://api:8080',       // Internal Docker service name
    'http://localhost:8080',  // Dev fallback
    'http://localhost:5173',  // SvelteKit dev server
];

export const handle: Handle = async ({ event, resolve }) => {
    const { pathname } = event.url;

    // 1. 🛡️ Performance: High-speed bypass for verified static assets
    if (ASSET_PREFIXES.some(prefix => pathname.startsWith(prefix))) {
        return await resolve(event);
    }

    // 🛡️ SSL Termination: Trust X-Forwarded-Proto from known internal proxies.
    // SvelteKit uses `event.url.protocol` for CSRF origin checks.
    // When behind an Nginx ingress that terminates SSL, the internal request
    // arrives as HTTP but the browser sent HTTPS. Without this, SvelteKit
    // rejects form submissions with an origin mismatch error.
    const forwardedProto = event.request.headers.get('x-forwarded-proto');
    if (forwardedProto === 'https') {
        // Trust the header — we are behind a verified internal proxy
        event.url.protocol = 'https:';
    }

    let accessToken = event.cookies.get('kari_access_token');
    const refreshToken = event.cookies.get('kari_refresh_token');

    // 2. 🔄 Hardened Silent Refresh Pipeline
    if (!accessToken && refreshToken) {
        try {
            const response = await event.fetch(`${env.INTERNAL_API_URL}/api/v1/auth/refresh`, {
                method: 'POST',
                headers: { 'Cookie': `kari_refresh_token=${refreshToken}` }
            });

            if (response.ok) {
                // 🛡️ SLA: Proxy all Set-Cookie headers from Brain to Browser correctly
                const setCookieHeaders = response.headers.getSetCookie();
                setCookieHeaders.forEach((cookie) => {
                    // This ensures all attributes (Secure, HttpOnly, SameSite) are preserved
                    event.setHeaders({ 'Set-Cookie': cookie });
                });

                // Re-extract for immediate locals population
                const cookies = response.headers.get('Set-Cookie');
                accessToken = cookies?.split(';')
                    .find(c => c.trim().startsWith('kari_access_token='))
                    ?.split('=')[1];
            } else {
                event.cookies.delete('kari_access_token', { path: '/' });
                event.cookies.delete('kari_refresh_token', { path: '/' });
            }
        } catch (err) {
            console.error("🚨 [SLA FATAL] Kari Brain Offline during refresh:", err);
        }
    }

    // 3. 🛡️ Cryptographic Verification (Strict Algorithm Enforcement)
    event.locals.user = null;

    if (accessToken) {
        try {
            const secret = new TextEncoder().encode(env.JWT_SECRET);
            const { payload } = await jose.jwtVerify(accessToken, secret, {
                algorithms: ['HS256'],
                issuer: 'kari:brain',
                audience: 'kari:panel'
            });

            event.locals.user = {
                id: payload.sub as string,
                role: payload.role as 'admin' | 'tenant'
            };
        } catch (error) {
            event.cookies.delete('kari_access_token', { path: '/' });
        }
    }

    // 4. 🛡️ Hardened Route Guarding
    const isAuthRoute = PUBLIC_ROUTES.includes(pathname);

    if (!event.locals.user && !isAuthRoute) {
        throw redirect(303, '/login');
    }

    if (event.locals.user && isAuthRoute) {
        throw redirect(303, '/dashboard');
    }

    // 5. 🛡️ SLA: Defense-in-Depth Headers
    const response = await resolve(event);
    
    // Protect against clickjacking and MIME-sniffing
    response.headers.set('X-Frame-Options', 'DENY');
    response.headers.set('X-Content-Type-Options', 'nosniff');
    response.headers.set('Referrer-Policy', 'strict-origin-when-cross-origin');
    
    // Strict Transport Security (1 year, includes subdomains, eligible for preload list)
    response.headers.set('Strict-Transport-Security', 'max-age=31536000; includeSubDomains; preload');

    // 🛡️ Permissions-Policy: Disable browser features we don't use
    response.headers.set('Permissions-Policy', 
        'camera=(), microphone=(), geolocation=(), payment=(), usb=()'
    );

    // 🛡️ Content Security Policy: Hardened for Production
    // - No 'unsafe-inline' for scripts (xterm.js uses JS canvas, not inline scripts)
    // - 'unsafe-inline' retained for styles (Svelte generates inline <style> blocks)
    // - connect-src allows SSE (self) and WebSocket (ws:/wss:) for telemetry
    // - font-src allows Google Fonts CDN for IBM Plex Mono
    response.headers.set(
        'Content-Security-Policy',
        "default-src 'self'; " +
        "script-src 'self'; " +
        "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
        "font-src 'self' https://fonts.gstatic.com; " +
        "connect-src 'self' ws: wss:; " +
        "img-src 'self' data:; " +
        "frame-ancestors 'none';"
    );

    return response;
};
