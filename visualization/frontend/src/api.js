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
  replaySources: () => request("/api/replay/sources"),
  createReplaySource: (payload) => request("/api/replay/sources", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) }),
  deleteReplaySource: (id) => request("/api/replay/sources/" + encodeURIComponent(id), { method: "DELETE" }),
  replayRecords: (source = "primary", fsmId = "") => request("/api/replay/records?source=" + encodeURIComponent(source) + "&fsmId=" + encodeURIComponent(fsmId) + "&limit=500"),
  replayRecord: (source, id) => request("/api/replay/records/" + encodeURIComponent(id) + "?source=" + encodeURIComponent(source)),
  deleteReplayRecord: (source, id) => request("/api/replay/records/" + encodeURIComponent(id) + "?source=" + encodeURIComponent(source), { method: "DELETE" }),
}
