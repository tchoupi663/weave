import { useEffect, useState } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { getToken } from '../auth'
import { clearCart, formatPrice } from '../cart'
import type { Order } from '../types'

export function Payment() {
  const token = getToken()
  const [params] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const orderId = params.get('orderId')
  const state = location.state as { amount?: number } | null

  const [order, setOrder] = useState<Order | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token || !orderId || state?.amount !== undefined) return
    let cancelled = false
    api
      .getOrder(orderId)
      .then((value) => {
        if (!cancelled) setOrder(value)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load the order.')
      })
    return () => {
      cancelled = true
    }
  }, [orderId, state?.amount, token])

  if (!token) {
    const next = orderId ? `/payment?orderId=${orderId}` : '/payment'
    return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />
  }

  if (!orderId) {
    return (
      <section className="panel narrow">
        <p className="eyebrow">Payment</p>
        <h1>Missing order</h1>
        <p className="muted">Start again from your cart.</p>
        <Link className="btn" to="/cart">
          Back to cart
        </Link>
      </section>
    )
  }

  const amount = state?.amount ?? order?.total ?? 0

  const pay = async () => {
    setBusy(true)
    setError('')
    try {
      await api.pay(orderId, amount)
      clearCart()
      navigate(`/orders/${orderId}`, { state: { paid: true } })
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Payment failed.')
      setBusy(false)
    }
  }

  return (
    <section className="panel narrow">
      <p className="eyebrow">Payment</p>
      <h1>Pay {amount > 0 ? formatPrice(amount) : ''}</h1>
      <p className="muted">Order {orderId}</p>
      <label className="field">
        <span>Card number</span>
        <input value="4242 4242 4242 4242" readOnly />
      </label>
      <div className="card-row">
        <label className="field">
          <span>Expiry</span>
          <input value="12 / 30" readOnly />
        </label>
        <label className="field">
          <span>CVC</span>
          <input value="123" readOnly />
        </label>
      </div>
      {error && <p className="error">{error}</p>}
      <button className="btn btn-block" type="button" onClick={pay} disabled={busy || amount <= 0}>
        {busy ? 'Paying…' : 'Pay now'}
      </button>
      <p className="fineprint">Simulated payment. No card is charged.</p>
    </section>
  )
}
