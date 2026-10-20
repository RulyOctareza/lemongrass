// Compact relative age for a unix timestamp in seconds.
export function formatAge(time: number): string {
  const minutes = Math.max(0, Math.round((Date.now() / 1000 - time) / 60))
  if (minutes < 60) return `${minutes}m`
  if (minutes < 60 * 24) return `${Math.round(minutes / 60)}h`
  if (minutes < 60 * 24 * 60) return `${Math.round(minutes / (60 * 24))}d`
  return `${Math.round(minutes / (60 * 24 * 30))}mo`
}

export function formatDate(time: number): string {
  return new Date(time * 1000).toLocaleString()
}
