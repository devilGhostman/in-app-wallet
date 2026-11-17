import { SignJWT } from 'jose'
import { env } from './env'

const DEMO_SCOPES = [
  'wallet:create',
  'wallet:read',
  'order:create',
  'order:spend',
  'order:read',
  'order:pay_simulate',
].join(' ')

let cached: { token: string; expMs: number } | null = null

/** Dev-only HS256 mint. Production should use a BFF. */
export async function getAccessToken(): Promise<string> {
  const now = Date.now()
  if (cached && cached.expMs - now > 60_000) {
    return cached.token
  }

  const secret = new TextEncoder().encode(env.jwtSecret)
  const exp = Math.floor(now / 1000) + 60 * 60
  const token = await new SignJWT({})
    .setProtectedHeader({ alg: 'HS256' })
    .setExpirationTime(exp)
    .sign(secret)

  cached = { token, expMs: exp * 1000 }
  return token
}

export function getDemoScopes(): string {
  return DEMO_SCOPES
}
