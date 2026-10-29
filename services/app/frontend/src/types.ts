export type Product = {
  id: string
  name: string
  price: number
}

export type CartItem = {
  productId: string
  name: string
  price: number
  quantity: number
}

export type OrderItem = {
  productId: string
  quantity: number
}

export type Order = {
  orderId: string
  status: string
  total: number
  items: OrderItem[]
  createdAt?: string
}
