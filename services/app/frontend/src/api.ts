import { clearSession, getToken } from './auth'
import type { Order, OrderItem, Product } from './types'

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body) {
    headers.set('Content-Type', 'application/json')
  }
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(path, { ...init, headers })
  if (!response.ok) {
    if (response.status === 401 && token) {
      clearSession()
    }
    let message = response.statusText
    try {
      const data = (await response.json()) as { error?: string }
      if (data.error) message = data.error
    } catch {
      // Keep the status text.
    }
    throw new ApiError(message || `Request failed (${response.status})`, response.status)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export const api = {
  products: () => request<Product[]>('/api/products'),
  product: (id: string) => request<Product>(`/api/products/${id}`),
  login: (username: string, password: string) =>
    request<{ token: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  createOrder: (items: OrderItem[], total: number) =>
    request<Order>('/api/orders', {
      method: 'POST',
      body: JSON.stringify({ items, total }),
    }),
  getOrder: (id: string) => request<Order>(`/api/orders/${id}`),
  pay: (orderId: string, amount: number) =>
    request<{ paymentId: string; status: string }>('/api/payments', {
      method: 'POST',
      body: JSON.stringify({ orderId, amount, card: '4242424242424242' }),
    }),
}
