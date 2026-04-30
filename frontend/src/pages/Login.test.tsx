import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'
import Login from './Login'
import { AuthProvider } from '../hooks/useAuth'

// Mock api module
vi.mock('@/lib/api', () => ({
  default: {
    post: vi.fn(),
  },
}))

// Import after mocking
import api from '@/lib/api'
const mockedApi = api as unknown as typeof api & {
  post: ReturnType<typeof vi.fn>
}

describe('Login', () => {
  it('renders login form with tabs', () => {
    render(
      <BrowserRouter>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </BrowserRouter>
    )

    // Check for tab buttons (there are two: "Sign In" and "Request Access")
    // We use getAllByRole to get both buttons
    const tabButtons = screen.getAllByRole('button')
    // Find the ones that look like tabs (by checking for specific text)
    const signInTab = tabButtons.find(btn => btn.textContent?.trim() === 'Sign In')
    const requestAccessTab = tabButtons.find(btn => btn.textContent?.trim() === 'Request Access')
    expect(signInTab).toBeInTheDocument()
    expect(requestAccessTab).toBeInTheDocument()
    
    // Check for form elements (default tab is "Sign In")
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
  })

  it('shows error message on invalid credentials', async () => {
    mockedApi.post.mockRejectedValueOnce({
      response: { status: 401, data: 'Invalid credentials' }
    })

    render(
      <BrowserRouter>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </BrowserRouter>
    )

    const emailInput = screen.getByLabelText(/email/i)
    const passwordInput = screen.getByLabelText(/password/i)
    // Find the submit button (there are multiple buttons with "Sign In" text)
    const buttons = screen.getAllByRole('button')
    const submitButton = buttons.find(btn => btn.textContent?.trim() === 'Sign in')
    expect(submitButton).toBeDefined()

    await userEvent.type(emailInput, 'test@example.com')
    await userEvent.type(passwordInput, 'wrongpassword')
    await userEvent.click(submitButton!)

    await waitFor(() => {
      expect(screen.getByText(/invalid credentials/i)).toBeInTheDocument()
    })
  })

  it('submits form with valid credentials', async () => {
    mockedApi.post.mockResolvedValueOnce({
      data: { token: 'fake-jwt-token' }
    })

    render(
      <BrowserRouter>
        <AuthProvider>
          <Login />
        </AuthProvider>
      </BrowserRouter>
    )

    const emailInput = screen.getByLabelText(/email/i)
    const passwordInput = screen.getByLabelText(/password/i)
    // Find the submit button (there are multiple buttons with "Sign In" text)
    const buttons = screen.getAllByRole('button')
    const submitButton = buttons.find(btn => btn.textContent?.trim() === 'Sign in')
    expect(submitButton).toBeDefined()

    await userEvent.type(emailInput, 'test@example.com')
    await userEvent.type(passwordInput, 'correctpassword')
    await userEvent.click(submitButton!)

    await waitFor(() => {
      expect(mockedApi.post).toHaveBeenCalledWith('/api/auth/login', {
        email: 'test@example.com',
        password: 'correctpassword'
      })
    })
  })
})