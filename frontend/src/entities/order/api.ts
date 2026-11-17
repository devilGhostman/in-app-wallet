import { http, unwrapData } from '@/shared/api/client'
import type { ApiError } from '@/shared/api/errors'
import type { Order, SimulatePaymentResult } from './model'

export async function createPurchaseOrder(input: {
  userId: string
  amount: number
  idempotencyKey: string
  metadata?: Record<string, unknown>
}): Promise<Order> {
  return unwrapData<Order>(
    http.post('/api/v1/internal/orders', {
      user_id: input.userId,
      amount: input.amount,
      idempotency_key: input.idempotencyKey,
      metadata: input.metadata,
    }),
  )
}

export async function createSpendOrder(input: {
  userId: string
  amount: number
  idempotencyKey: string
  metadata?: Record<string, unknown>
}): Promise<Order> {
  return unwrapData<Order>(
    http.post('/api/v1/internal/orders/spend', {
      user_id: input.userId,
      amount: input.amount,
      idempotency_key: input.idempotencyKey,
      metadata: input.metadata,
    }),
  )
}

export async function simulatePayment(orderId: string): Promise<SimulatePaymentResult> {
  return unwrapData<SimulatePaymentResult>(
    http.post('/api/v1/internal/payments/simulate', { order_id: orderId }),
  )
}

export async function getOrder(orderId: string): Promise<Order> {
  return unwrapData<Order>(
    http.get(`/api/v1/internal/orders/${encodeURIComponent(orderId)}`),
  )
}

export async function pollOrderTerminal(
  orderId: string,
  opts: { timeoutMs?: number; intervalMs?: number } = {},
): Promise<Order> {
  const timeoutMs = opts.timeoutMs ?? 30_000
  const intervalMs = opts.intervalMs ?? 800
  const deadline = Date.now() + timeoutMs

  for (;;) {
    const order = await getOrder(orderId)
    if (order.status === 'completed' || order.status === 'failed') {
      return order
    }
    if (Date.now() >= deadline) {
      throw {
        status: 408,
        message: 'Timed out waiting for order to complete',
      } satisfies ApiError
    }
    await new Promise((r) => setTimeout(r, intervalMs))
  }
}
