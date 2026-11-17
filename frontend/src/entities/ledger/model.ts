export type LedgerEntryType = 'CREDIT' | 'DEBIT'

export type LedgerItem = {
  ledger_id: string
  transaction_id: string
  app_id: string
  entry_type: LedgerEntryType | string
  reason: string
  amount: number
  balance_before: number
  balance_after: number
  order_id: string
  idempotency_key: string
  created_at: string
  metadata?: Record<string, unknown>
}

export type LedgerPage = {
  transactions: LedgerItem[]
  next_cursor?: string
}
