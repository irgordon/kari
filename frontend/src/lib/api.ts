export type ApiPath = `/api/v1/${string}`

export interface AccountUser { id: string; email: string; email_verified: boolean; mfa_enabled: boolean; mfa_required: boolean }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}

let refresh: Promise<Response> | null = null

async function renewSession(): Promise<Response> {
  // Another tab may already have renewed the shared cookies while we waited.
  let current = await fetch('/api/v1/auth/me', { credentials: 'include' })
  if (current.status === 429) {
    await new Promise(resolve => window.setTimeout(resolve, 1000))
    current = await fetch('/api/v1/auth/me', { credentials: 'include' })
  }
  if (current.status !== 401) return current
  let renewed = await fetch('/api/v1/auth/refresh', { method: 'POST', credentials: 'include' })
  if (renewed.status === 429) {
    await new Promise(resolve => window.setTimeout(resolve, 1000))
    renewed = await fetch('/api/v1/auth/refresh', { method: 'POST', credentials: 'include' })
  }
  return renewed
}

async function request<TResponse>(
  path: ApiPath,
  init: RequestInit = {},
): Promise<TResponse> {
  const options = {
    credentials: 'include',
    ...init,
  } satisfies RequestInit
  let response = path === '/api/v1/auth/logout' && navigator.locks
    ? await navigator.locks.request('kari-session-refresh', () => fetch(path, options))
    : await fetch(path, options)
  // Retry safe reads once after brief throttling; never hide a persistent failure.
  if (response.status === 429 && options.method === 'GET') {
    await new Promise(resolve => window.setTimeout(resolve, 1000))
    response = await fetch(path, options)
  }
  if (response.status === 401 && !path.startsWith('/api/v1/setup/') && !['/api/v1/auth/login', '/api/v1/auth/refresh', '/api/v1/auth/logout', '/api/v1/auth/mfa/complete'].includes(path)) {
    // Coordinate tabs on secure origins and share pending renewal within this tab.
    refresh ??= (navigator.locks ? navigator.locks.request('kari-session-refresh', renewSession) : renewSession()).finally(() => { refresh = null })
    const renewal = await refresh
    if (renewal.ok) response = await fetch(path, options)
    else if (renewal.status !== 401) throw new ApiError(renewal.status, 'Sign-in service unavailable. Please try again.')
    if (response.status === 401) window.dispatchEvent(new Event('kari-session-expired'))
  }

  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { message?: string }
    throw new ApiError(response.status, body.message ?? `Request failed (${response.status}). Please try again.`)
  }

  if (response.status === 204) {
    return undefined as TResponse
  }

  return (await response.json()) as TResponse
}

export function apiGet<TResponse>(
  path: ApiPath,
  init: RequestInit = {},
): Promise<TResponse> {
  return request<TResponse>(path, {
    method: 'GET',
    ...init,
  })
}

export function apiPost<TRequest, TResponse>(
  path: ApiPath,
  body: TRequest,
  init: RequestInit = {},
): Promise<TResponse> {
  return request<TResponse>(path, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
    body: JSON.stringify(body),
    ...init,
  })
}

export function apiPut<TRequest, TResponse>(
  path: ApiPath,
  body: TRequest,
  init: RequestInit = {},
): Promise<TResponse> {
  return request<TResponse>(path, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers ?? {}),
    },
    body: JSON.stringify(body),
    ...init,
  })
}

export function apiDelete<TResponse = void>(
  path: ApiPath,
  init: RequestInit = {},
): Promise<TResponse> {
  return request<TResponse>(path, {
    method: 'DELETE',
    ...init,
  })
}
