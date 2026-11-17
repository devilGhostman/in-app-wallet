export type OrderStatus = 'pending' | 'paid' | 'completed' | 'failed' | 'cancelled' | string

export type Order = {
  order_id: string
  app_id: string
  user_id: string
  amount: number
  type: string
  status: OrderStatus
  idempotency_key: string
  payment_ref?: string
  failure_code?: string
  failure_message?: string
  metadata?: Record<string, unknown>
  paid_at?: string
  created_at: string
}

export type SimulatePaymentResult = {
  order_id: string
  payment_ref: string
  status: string
  webhook_ok: boolean
}
