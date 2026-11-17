function required(name: keyof ImportMetaEnv): string {
  const value = import.meta.env[name]
  if (typeof value !== 'string' || !value.trim()) {
    throw new Error(`Missing required env ${name}. Copy .env.example to .env and set it.`)
  }
  return value.trim()
}

function withDefault(name: keyof ImportMetaEnv, fallback: string): string {
  const value = import.meta.env[name]
  if (typeof value === 'string' && value.trim()) {
    return value.trim().replace(/\/$/, '')
  }
  return fallback
}

export const env = {
  appId: required('VITE_APP_ID'),
  jwtSecret: required('VITE_JWT_SECRET'),
  apiUrl: withDefault('VITE_API_URL', 'http://localhost:8081'),
} as const
