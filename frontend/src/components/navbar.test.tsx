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

  it('opens mobile menu when hamburger button is clicked', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    expect(mobileMenu).toBeInTheDocument()
  })

  it('mobile dropdown menu has opaque background without backdrop-blur', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    const style = mobileMenu.getAttribute('style')
    expect(style).toContain('background-color')
    expect(style).toContain('hsl(var(--color-background))')
    expect(mobileMenu.className).not.toContain('/95')
    expect(mobileMenu.className).not.toContain('/60')
    expect(mobileMenu.className).not.toContain('backdrop-blur')
  })

  it('mobile dropdown menu has shadow for visual separation', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    expect(mobileMenu.className).toContain('shadow-lg')
  })

  it('mobile dropdown menu closes when a link is clicked', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    expect(mobileMenu).toBeInTheDocument()

    const collectionsLink = screen.getByTestId('mobile-collections-link')
    fireEvent.click(collectionsLink)

    expect(screen.queryByTestId('mobile-dropdown-menu')).not.toBeInTheDocument()
  })

  it('mobile dropdown menu has solid border-bottom for contrast', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    expect(mobileMenu.className).toContain('border-b')
  })

  it('navbar does not use backdrop-blur which causes transparency on mobile', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const nav = screen.getByRole('navigation')
    expect(nav.className).not.toContain('backdrop-blur')
  })

  it('mobile dropdown menu is right-aligned under the hamburger icon', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    const classes = mobileMenu.className
    expect(classes).toContain('right-0')
    expect(classes).not.toContain('left-0')
  })

  it('mobile dropdown menu items are right-justified', () => {
    render(
      <BrowserRouter>
        <Navbar />
      </BrowserRouter>
    )

    const hamburgerButton = screen.getByRole('button', { name: /menu/i })
    fireEvent.click(hamburgerButton)

    const mobileMenu = screen.getByTestId('mobile-dropdown-menu')
    const innerDiv = mobileMenu.querySelector('.flex')
    expect(innerDiv?.className).toContain('text-right')
  })
})