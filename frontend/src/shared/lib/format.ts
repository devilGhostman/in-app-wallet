export function formatCoins(amount: number): string {
  return amount.toLocaleString()
}

export function formatReason(reason: string): string {
  return reason.replaceAll('_', ' ')
}

export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

export function truncateId(id: string, head = 8): string {
  if (id.length <= head) return id
  return `${id.slice(0, head)}…`
}
