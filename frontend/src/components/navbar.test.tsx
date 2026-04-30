import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'

// Mock useAuth before importing Navbar
vi.mock('@/hooks/useAuth', () => ({
  useAuth: vi.fn(() => ({ user: { role: 'admin' }, isAuthenticated: true })),
}))

import { Navbar } from './navbar'

describe('Navbar', () => {
  it('renders navigation links for admin users', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    expect(screen.getByText(/collections/i)).toBeInTheDocument()
    expect(screen.getByText(/admin/i)).toBeInTheDocument()
    expect(screen.getByText(/account/i)).toBeInTheDocument()
  })

  it('navigates to collections page when Collections link is clicked', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const collectionsLink = screen.getByText(/collections/i)
    fireEvent.click(collectionsLink)

    // The link should have the correct href
    expect(collectionsLink.closest('a')).toHaveAttribute('href', '/collections?reset=true')
  })

  it('navigates to admin page when Admin link is clicked', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const adminLink = screen.getByText(/admin/i)
    fireEvent.click(adminLink)

    // The link should have the correct href
    expect(adminLink.closest('a')).toHaveAttribute('href', '/admin')
  })

  it('displays theme toggle button', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    expect(screen.getByRole('button', { name: /toggle theme/i })).toBeInTheDocument()
  })
})