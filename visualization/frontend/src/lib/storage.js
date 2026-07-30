export function stored(key, fallback) {
  try { return localStorage.getItem(key) ?? fallback } catch { return fallback }
}

export function remember(key, value) {
  try { localStorage.setItem(key, String(value)) } catch { /* storage is optional */ }
}
