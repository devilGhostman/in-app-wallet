import axios, { type AxiosError } from 'axios'
import type { ApiEnvelope } from './types'

export type ApiError = {
  status: number
  code?: string
  message: string
}

export function toApiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const ax = error as AxiosError<ApiEnvelope<unknown>>
    const body = ax.response?.data
    return {
      status: ax.response?.status ?? 0,
      code: body?.code,
      message: body?.message || ax.message || 'Request failed',
    }
  }

  if (error && typeof error === 'object' && 'message' in error) {
    const maybe = error as Partial<ApiError>
    return {
      status: maybe.status ?? 0,
      code: maybe.code,
      message: String(maybe.message),
    }
  }

  return { status: 0, message: 'Request failed' }
}

export function isNotFound(error: ApiError): boolean {
  return error.status === 404 || error.code === 'NOT_FOUND'
}

export function isConflict(error: ApiError): boolean {
  return error.status === 409 || error.code === 'ALREADY_EXISTS'
}
