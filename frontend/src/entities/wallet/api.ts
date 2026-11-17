import { http, unwrapData } from '@/shared/api/client'
import { isConflict, isNotFound, toApiError } from '@/shared/api/errors'
import type { Wallet } from './model'

export async function createWallet(userId: string): Promise<Wallet> {
  return unwrapData<Wallet>(http.post('/api/v1/internal/wallets', { user_id: userId }))
}

export async function getWallet(userId: string): Promise<Wallet> {
  return unwrapData<Wallet>(
    http.get(`/api/v1/internal/wallets/${encodeURIComponent(userId)}`),
  )
}

/** GET first; POST create only when missing. */
export async function ensureWallet(userId: string): Promise<Wallet> {
  try {
    return await getWallet(userId)
  } catch (e) {
    const err = toApiError(e)
    if (!isNotFound(err)) throw err

    try {
      return await createWallet(userId)
    } catch (createErr) {
      const c = toApiError(createErr)
      if (isConflict(c)) return getWallet(userId)
      throw c
    }
  }
}
