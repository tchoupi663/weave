import { useState } from 'react'
import { Link } from 'react-router-dom'
import { cartTotal, formatPrice, loadCart, removeFromCart, updateQuantity } from '../cart'
import type { CartItem } from '../types'

export function Cart() {
  const [items, setItems] = useState<CartItem[]>(loadCart)

  const refresh = () => setItems(loadCart())

  const changeQuantity = (productId: string, quantity: number) => {
    updateQuantity(productId, quantity)
    refresh()
  }

  const remove = (productId: string) => {
    removeFromCart(productId)
    refresh()
  }

  if (items.length === 0) {
    return (
      <section className="panel narrow">
        <p className="eyebrow">Cart</p>
        <h1>Nothing here yet</h1>
        <p className="muted">Add a product and it will show up here.</p>
        <Link className="btn" to="/products">
          Browse products
        </Link>
      </section>
    )
  }

  const total = cartTotal(items)

  return (
    <section>
      <div className="section-head">
        <div>
          <p className="eyebrow">Cart</p>
          <h1>Your selection</h1>
        </div>
        <Link className="link" to="/products">
          Continue shopping
        </Link>
      </div>

      <div className="cart-layout">
        <div className="cart-lines">
          {items.map((item) => (
            <article className="cart-line" key={item.productId}>
              <div className="line-art" aria-hidden="true" />
              <div className="line-copy">
                <span className="product-id">{item.productId}</span>
                <h2>{item.name}</h2>
                <span className="muted">{formatPrice(item.price)} each</span>
              </div>
              <div className="qty">
                <button
                  type="button"
                  aria-label={`Decrease ${item.name}`}
                  onClick={() => changeQuantity(item.productId, item.quantity - 1)}
                >
                  −
                </button>
                <span>{item.quantity}</span>
                <button
                  type="button"
                  aria-label={`Increase ${item.name}`}
                  onClick={() => changeQuantity(item.productId, item.quantity + 1)}
                >
                  +
                </button>
              </div>
              <span className="line-total">{formatPrice(item.price * item.quantity)}</span>
              <button className="remove" type="button" onClick={() => remove(item.productId)}>
                Remove
              </button>
            </article>
          ))}
        </div>

        <aside className="summary">
          <h2>Order summary</h2>
          <div className="summary-row">
            <span>Subtotal</span>
            <span>{formatPrice(total)}</span>
          </div>
          <div className="summary-row">
            <span>Shipping</span>
            <span>Free</span>
          </div>
          <div className="summary-row total">
            <span>Total</span>
            <span>{formatPrice(total)}</span>
          </div>
          <Link className="btn btn-block" to="/checkout">
            Checkout
          </Link>
          <p className="fineprint">Payment is simulated. Use any card details on the next step.</p>
        </aside>
      </div>
    </section>
  )
}
