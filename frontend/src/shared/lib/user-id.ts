const STORAGE_KEY = 'inapp_wallet_user_id'
const USER_ID_RE = /^[a-fA-F0-9]{24}$/

/** 24-char hex id (Mongo ObjectId style). */
export function generateUserId(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function getOrCreateUserId(): string {
  const existing = localStorage.getItem(STORAGE_KEY)?.trim()
  if (existing && USER_ID_RE.test(existing)) {
    return existing
  }
  const id = generateUserId()
  localStorage.setItem(STORAGE_KEY, id)
  return id
}

export function shortUserId(userId: string): string {
  if (userId.length <= 10) return userId
  return `${userId.slice(0, 4)}…${userId.slice(-4)}`
}
