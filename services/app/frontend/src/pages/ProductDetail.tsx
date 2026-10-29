import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api } from '../api'
import { addToCart, formatPrice } from '../cart'
import type { Product } from '../types'

function artStyle(index: number) {
  const hue = (index * 47 + 18) % 360
  return {
    background: `linear-gradient(135deg, hsl(${hue} 58% 74%), hsl(${(hue + 42) % 360} 48% 52%))`,
  }
}

export function ProductDetail() {
  const { id } = useParams<{ id: string }>()
  const [product, setProduct] = useState<Product | null>(null)
  const [error, setError] = useState('')
  const [added, setAdded] = useState(false)

  useEffect(() => {
    if (!id) {
      setError('Missing product id.')
      return
    }
    let cancelled = false
    api
      .product(id)
      .then((item) => {
        if (!cancelled) setProduct(item)
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load product.')
      })
    return () => {
      cancelled = true
    }
  }, [id])

  if (error) {
    return (
      <section className="panel">
        <p className="eyebrow">Product</p>
        <h1>Not found</h1>
        <p className="muted">{error}</p>
        <Link className="btn" to="/products">
          Back to products
        </Link>
      </section>
    )
  }

  if (!product) {
    return (
      <section className="panel">
        <p className="muted">Loading product…</p>
      </section>
    )
  }

  const index = Number(product.id.replace('p', '')) || 0
  const add = () => {
    addToCart(product)
    setAdded(true)
  }

  return (
    <section>
      <nav className="crumbs">
        <Link to="/products">Products</Link>
        <span>/</span>
        <span>{product.id}</span>
      </nav>
      <div className="detail">
        <div className="detail-art" style={artStyle(index)}>
          <span>{product.name.slice(-2)}</span>
        </div>
        <div className="detail-copy">
          <p className="eyebrow">Catalog item</p>
          <h1>{product.name}</h1>
          <p className="price big">{formatPrice(product.price)}</p>
          <p className="muted">
            A dependable everyday object. This description is intentionally generic; the interesting
            part is the traffic it produces.
          </p>
          <div className="detail-actions">
            <button className="btn" type="button" onClick={add}>
              Add to cart
            </button>
            {added && (
              <Link className="btn btn-ghost" to="/cart">
                View cart
              </Link>
            )}
          </div>
          <dl className="facts">
            <div>
              <dt>SKU</dt>
              <dd>{product.id}</dd>
            </div>
            <div>
              <dt>Ships</dt>
              <dd>1–2 days</dd>
            </div>
            <div>
              <dt>Returns</dt>
              <dd>30 days</dd>
            </div>
          </dl>
        </div>
      </div>
    </section>
  )
}
