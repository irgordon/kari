export type ApiPath = `/api/v1/${string}`

async function request<TResponse>(
  path: ApiPath,
  init: RequestInit = {},
): Promise<TResponse> {
  const response = await fetch(path, {
    credentials: 'include',
    ...init,
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(`API request failed (${response.status}): ${message}`)
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
