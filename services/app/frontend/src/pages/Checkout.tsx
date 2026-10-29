import { useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { api } from '../api'
import { getToken } from '../auth'
import { cartTotal, clearCart, formatPrice, loadCart } from '../cart'

export function Checkout() {
  const token = getToken()
  const navigate = useNavigate()
  const [items] = useState(loadCart)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  if (!token) {
    return <Navigate to={`/login?next=${encodeURIComponent('/checkout')}`} replace />
  }

  if (items.length === 0) {
    return (
      <section className="panel narrow">
        <p className="eyebrow">Checkout</p>
        <h1>Your cart is empty</h1>
        <Link className="btn" to="/products">
          Browse products
        </Link>
      </section>
    )
  }

  const total = cartTotal(items)

  const placeOrder = async () => {
    setBusy(true)
    setError('')
    try {
      const order = await api.createOrder(
        items.map((item) => ({ productId: item.productId, quantity: item.quantity })),
        total,
      )
      clearCart()
      navigate(`/payment?orderId=${order.orderId}`, { state: { amount: order.total } })
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Could not create the order.')
      setBusy(false)
    }
  }

  return (
    <section>
      <div className="section-head">
        <div>
          <p className="eyebrow">Checkout</p>
          <h1>Review your order</h1>
        </div>
        <Link className="link" to="/cart">
          Edit cart
        </Link>
      </div>

      <div className="checkout-layout">
        <div className="panel">
          <h2>Items</h2>
          <ul className="plain-list">
            {items.map((item) => (
              <li key={item.productId}>
                <span>
                  {item.name} <span className="muted">× {item.quantity}</span>
                </span>
                <span>{formatPrice(item.price * item.quantity)}</span>
              </li>
            ))}
          </ul>
          <div className="summary-row total">
            <span>Total</span>
            <span>{formatPrice(total)}</span>
          </div>
          {error && <p className="error">{error}</p>}
          <button className="btn btn-block" type="button" onClick={placeOrder} disabled={busy}>
            {busy ? 'Placing order…' : 'Place order'}
          </button>
          <p className="fineprint">
            This creates an order and takes you to the simulated payment page.
          </p>
        </div>

        <aside className="summary">
          <h2>Delivery</h2>
          <p className="muted">
            Weave Test Lane 1<br />
            00000 Localhost
          </p>
          <h2>Payment</h2>
          <p className="muted">Simulated card ending 4242</p>
        </aside>
      </div>
    </section>
  )
}
