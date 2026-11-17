import { http, unwrapData } from '@/shared/api/client'
import type { LedgerEntryType, LedgerPage } from './model'

export type ListLedgerParams = {
  entryType?: LedgerEntryType
  limit?: number
  cursor?: string
}

export async function listLedger(userId: string, params: ListLedgerParams = {}): Promise<LedgerPage> {
  return unwrapData<LedgerPage>(
    http.get(`/api/v1/internal/wallets/${encodeURIComponent(userId)}/ledger`, {
      params: {
        entry_type: params.entryType,
        limit: params.limit,
        cursor: params.cursor,
      },
    }),
  )
}
