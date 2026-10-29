import type { CartItem, Product } from './types'

const CART_KEY = 'weave.cart'

export function loadCart(): CartItem[] {
  try {
    const raw = localStorage.getItem(CART_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as CartItem[]) : []
  } catch {
    return []
  }
}

export function saveCart(items: CartItem[]): void {
  localStorage.setItem(CART_KEY, JSON.stringify(items))
  window.dispatchEvent(new Event('weave-cart'))
}

export function addToCart(product: Product, quantity = 1): void {
  const items = loadCart()
  const existing = items.find((item) => item.productId === product.id)
  if (existing) {
    existing.quantity += quantity
  } else {
    items.push({
      productId: product.id,
      name: product.name,
      price: product.price,
      quantity,
    })
  }
  saveCart(items)
}

export function updateQuantity(productId: string, quantity: number): void {
  const items = loadCart()
    .map((item) => (item.productId === productId ? { ...item, quantity } : item))
    .filter((item) => item.quantity > 0)
  saveCart(items)
}

export function removeFromCart(productId: string): void {
  saveCart(loadCart().filter((item) => item.productId !== productId))
}

export function clearCart(): void {
  saveCart([])
}

export function cartCount(items: CartItem[]): number {
  return items.reduce((count, item) => count + item.quantity, 0)
}

export function cartTotal(items: CartItem[]): number {
  return Math.round(items.reduce((total, item) => total + item.price * item.quantity, 0) * 100) / 100
}

export function formatPrice(value: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(value)
}
