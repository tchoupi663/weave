import { useEffect, useState } from 'react'
import { Link, NavLink, Route, Routes, useNavigate } from 'react-router-dom'
import { clearSession, getToken, getUsername } from './auth'
import { cartCount, loadCart } from './cart'
import type { CartItem } from './types'
import { Landing } from './pages/Landing'
import { Products } from './pages/Products'
import { ProductDetail } from './pages/ProductDetail'
import { Cart } from './pages/Cart'
import { Checkout } from './pages/Checkout'
import { Payment } from './pages/Payment'
import { OrderConfirmation } from './pages/OrderConfirmation'
import { Login } from './pages/Login'
import { NotFound } from './pages/NotFound'

export function App() {
  const [cart, setCart] = useState<CartItem[]>(loadCart)
  const [token, setToken] = useState<string | null>(getToken)
  const [username, setUsername] = useState<string | null>(getUsername)
  const navigate = useNavigate()

  useEffect(() => {
    const sync = () => {
      setCart(loadCart())
      setToken(getToken())
      setUsername(getUsername())
    }
    window.addEventListener('weave-cart', sync)
    window.addEventListener('weave-auth', sync)
    window.addEventListener('storage', sync)
    return () => {
      window.removeEventListener('weave-cart', sync)
      window.removeEventListener('weave-auth', sync)
      window.removeEventListener('storage', sync)
    }
  }, [])

  const count = cartCount(cart)

  const signOut = () => {
    clearSession()
    navigate('/')
  }

  return (
    <div className="shell">
      <header className="site-header">
        <Link className="brand" to="/">
          Weave<span>Shop</span>
        </Link>
        <nav className="nav" aria-label="Main">
          <NavLink to="/" end>
            Home
          </NavLink>
          <NavLink to="/products">Products</NavLink>
          <NavLink to="/cart">
            Cart
            {count > 0 && <span className="pill">{count}</span>}
          </NavLink>
        </nav>
        <div className="session">
          {token ? (
            <>
              <span className="who">@{username}</span>
              <button className="link-button" onClick={signOut} type="button">
                Sign out
              </button>
            </>
          ) : (
            <Link className="btn btn-small" to="/login">
              Sign in
            </Link>
          )}
        </div>
      </header>

      <main className="main">
        <Routes>
          <Route path="/" element={<Landing />} />
          <Route path="/products" element={<Products />} />
          <Route path="/products/:id" element={<ProductDetail />} />
          <Route path="/cart" element={<Cart />} />
          <Route path="/checkout" element={<Checkout />} />
          <Route path="/payment" element={<Payment />} />
          <Route path="/orders/:id" element={<OrderConfirmation />} />
          <Route path="/login" element={<Login />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>

      <footer className="footer">
        <span>Weave Target App</span>
        <span>Dummy storefront for topology and telemetry tests.</span>
      </footer>
    </div>
  )
}
