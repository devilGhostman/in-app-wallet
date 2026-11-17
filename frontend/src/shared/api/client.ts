import axios, { type AxiosInstance } from 'axios'
import { getAccessToken, getDemoScopes } from '@/shared/config/auth'
import { env } from '@/shared/config/env'
import { toApiError, type ApiError } from './errors'
import type { ApiEnvelope } from './types'

function createClient(baseURL: string): AxiosInstance {
  const client = axios.create({
    baseURL,
    headers: { 'Content-Type': 'application/json' },
  })

  client.interceptors.request.use(async (config) => {
    const token = await getAccessToken()
    config.headers.set('Authorization', `Bearer ${token}`)
    config.headers.set('X-App-Id', env.appId)
    config.headers.set('X-Scopes', getDemoScopes())
    if (!config.headers.get('X-Request-ID')) {
      config.headers.set('X-Request-ID', crypto.randomUUID())
    }
    return config
  })

  client.interceptors.response.use(
    (res) => {
      const body = res.data as ApiEnvelope<unknown>
      if (body && typeof body === 'object' && 'success' in body && body.success === false) {
        return Promise.reject({
          status: res.status,
          code: body.code,
          message: body.message || 'Request failed',
        } satisfies ApiError)
      }
      return res
    },
    (error) => Promise.reject(toApiError(error)),
  )

  return client
}

/** Single axios client pointed at gateway-svc. */
export const http = createClient(env.apiUrl)

export async function unwrapData<T>(promise: Promise<{ data: ApiEnvelope<T> }>): Promise<T> {
  const { data } = await promise
  return data.data
}
