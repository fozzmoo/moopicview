import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'

// Mock auth hook before importing AdminDashboard
vi.mock('@/hooks/useAuth', () => ({
  useAuth: vi.fn(() => ({ user: { role: 'admin' }, isAuthenticated: true })),
}))

// Mock api module
vi.mock('@/lib/api', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}))

// Import after mocking
import AdminDashboard from './AdminDashboard'
import api from '@/lib/api'
const mockedApi = api as unknown as typeof api & {
  get: ReturnType<typeof vi.fn>
  post: ReturnType<typeof vi.fn>
  delete: ReturnType<typeof vi.fn>
}

describe('AdminDashboard', () => {
  it('renders admin dashboard with users and proposed edits', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, email: 'user1@example.com', name: 'User One', role: 'user', approved: true },
        { id: 2, email: 'user2@example.com', name: 'User Two', role: 'user', approved: false }
      ]
    }).mockResolvedValueOnce({
      data: [
        { id: 1, photo_id: 1, user_id: 3, user_email: 'user3@example.com', field: 'description', proposed_value: 'New description', current_value: 'Old description', status: 'pending' }
      ]
    }).mockResolvedValueOnce({
      data: []
    })

    render(
      <BrowserRouter>
        <AdminDashboard />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/Users/i)).toBeInTheDocument()
      expect(screen.getByText(/Proposed Edits/i)).toBeInTheDocument()
      expect(screen.getByText(/user1@example.com/i)).toBeInTheDocument()
      expect(screen.getByText(/user2@example.com/i)).toBeInTheDocument()
      expect(screen.getByText(/New description/i)).toBeInTheDocument()
    })
  })

  it('approves a user', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, email: 'user1@example.com', name: 'User One', role: 'user', approved: false }
      ]
    }).mockResolvedValueOnce({
      data: []
    }).mockResolvedValueOnce({
      data: []
    })

    mockedApi.post.mockResolvedValueOnce({})

    render(
      <BrowserRouter>
        <AdminDashboard />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/user1@example.com/i)).toBeInTheDocument()
    })

    const approveButton = screen.getByRole('button', { name: /approve/i })
    fireEvent.click(approveButton)

    await waitFor(() => {
      expect(mockedApi.post).toHaveBeenCalledWith('/api/admin/users/1/approve')
    })
  })

  it('rejects a proposed edit', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: []
    }).mockResolvedValueOnce({
      data: [
        { id: 1, photo_id: 1, user_id: 3, user_email: 'user3@example.com', field: 'description', proposed_value: 'New description', current_value: 'Old description', status: 'pending' }
      ]
    }).mockResolvedValueOnce({
      data: []
    })

    mockedApi.post.mockResolvedValueOnce({})

    render(
      <BrowserRouter>
        <AdminDashboard />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/New description/i)).toBeInTheDocument()
    })

    const rejectButton = screen.getByRole('button', { name: /reject/i })
    fireEvent.click(rejectButton)

    await waitFor(() => {
      expect(mockedApi.post).toHaveBeenCalledWith('/api/admin/proposed-edits/1/review', { status: 'rejected' })
    })
  })

  it('sends password reset email for a user', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, email: 'user1@example.com', name: 'User One', role: 'user', approved: true }
      ]
    }).mockResolvedValueOnce({
      data: []
    }).mockResolvedValueOnce({
      data: []
    })

    mockedApi.post.mockResolvedValueOnce({})

    render(
      <BrowserRouter>
        <AdminDashboard />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/user1@example.com/i)).toBeInTheDocument()
    })

    // Wait for the Reset Password button to appear
    await waitFor(() => {
      const resetButtons = screen.getAllByRole('button', { name: /reset password/i })
      expect(resetButtons.length).toBeGreaterThan(0)
    })

    // Find and click the Reset Password button
    const resetButton = screen.getByRole('button', { name: /reset password/i })
    fireEvent.click(resetButton)

    await waitFor(() => {
      expect(mockedApi.post).toHaveBeenCalledWith('/api/admin/users/1/reset-password')
    })
  })

  it('deletes a user', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, email: 'user1@example.com', name: 'User One', role: 'user', approved: true }
      ]
    }).mockResolvedValueOnce({
      data: []
    }).mockResolvedValueOnce({
      data: []
    })

    mockedApi.delete.mockResolvedValueOnce({})

    render(
      <BrowserRouter>
        <AdminDashboard />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/user1@example.com/i)).toBeInTheDocument()
    })

    // Find and click the Delete button
    const deleteButton = screen.getByRole('button', { name: /delete/i })
    fireEvent.click(deleteButton)

    // Confirm deletion in the dialog
    await waitFor(() => {
      expect(screen.getByText(/Are you sure you want to delete this user/i)).toBeInTheDocument()
    })

    const confirmButton = screen.getByRole('button', { name: /delete user/i })
    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(mockedApi.delete).toHaveBeenCalledWith('/api/admin/users/1/delete')
    })
  })
})