import { useEffect, useState } from 'react'
import { Link, Navigate, useLocation, useParams } from 'react-router-dom'
import { api } from '../api'
import { getToken } from '../auth'
import { formatPrice } from '../cart'
import type { Order } from '../types'

export function OrderConfirmation() {
  const token = getToken()
  const { id } = useParams<{ id: string }>()
  const location = useLocation()
  const paid = (location.state as { paid?: boolean } | null)?.paid === true

  const [order, setOrder] = useState<Order | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token || !id) return
    let cancelled = false
    api
      .getOrder(id)
      .then((value) => {
        if (!cancelled) setOrder(value)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load the order.')
      })
    return () => {
      cancelled = true
    }
  }, [id, token])

  if (!token) {
    const next = id ? `/orders/${id}` : '/orders'
    return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />
  }

  if (error) {
    return (
      <section className="panel narrow">
        <p className="eyebrow">Order</p>
        <h1>Order unavailable</h1>
        <p className="error">{error}</p>
        <Link className="btn" to="/products">
          Back to products
        </Link>
      </section>
    )
  }

  if (!order) {
    return (
      <section className="panel narrow">
        <p className="muted">Loading order…</p>
      </section>
    )
  }

  return (
    <section className="panel narrow">
      <div className={paid ? 'status-mark success' : 'status-mark'} aria-hidden="true">
        ✓
      </div>
      <p className="eyebrow">Order confirmed</p>
      <h1>{paid ? 'Payment succeeded' : `Order ${order.status}`}</h1>
      <p className="muted">Order {order.orderId}</p>
      <div className="summary-row total">
        <span>Total</span>
        <span>{formatPrice(order.total)}</span>
      </div>
      <p className="muted">{order.items.length} line item(s)</p>
      <div className="detail-actions">
        <Link className="btn" to="/products">
          Continue shopping
        </Link>
      </div>
    </section>
  )
}
