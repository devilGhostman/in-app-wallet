export const TOP_UP_AMOUNTS = [50, 100, 250, 500] as const

export type TopUpStep = 'amount' | 'pay' | 'processing' | 'done'
