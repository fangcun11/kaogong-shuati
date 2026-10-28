import { reactive } from 'vue'
import { api, post } from '../api/client'

export interface User {
  id: number
  username: string
  role: 'user' | 'admin'
  status: number
}

export const session = reactive({
  user: null as User | null,
  ready: false,
  loading: false,
})

export async function loadSession(): Promise<void> {
  if (session.loading) return
  session.loading = true
  try {
    const data = await api<{ user: User }>('/api/auth/me')
    session.user = data.user
  } catch {
    session.user = null
  } finally {
    session.ready = true
    session.loading = false
  }
}

export async function login(username: string, password: string): Promise<void> {
  await post('/api/auth/login', { username, password })
  await loadSession()
}

export async function logout(): Promise<void> {
  try {
    await post('/api/auth/logout', {})
  } finally {
    session.user = null
    session.ready = true
  }
}
