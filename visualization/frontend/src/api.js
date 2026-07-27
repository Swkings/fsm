async function request(path, options) {
  const response = await fetch(path, { cache: 'no-store', ...options })
  if (!response.ok) {
    const body = await response.json().catch(() => ({}))
    throw new Error(body.error || response.statusText || `HTTP ${response.status}`)
  }
  if (response.status === 204) return null
  return response.json()
}

export const api = {
  fsms: () => request('/api/fsms'),
  discoveries: () => request('/api/discoveries'),
  createDiscovery: (payload) => request('/api/discoveries', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  }),
  refreshDiscovery: (id) => request(`/api/discoveries/${encodeURIComponent(id)}/refresh`, { method: 'POST' }),
  deleteDiscovery: (id) => request(`/api/discoveries/${encodeURIComponent(id)}`, { method: 'DELETE' }),
}
