import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { BrowserRouter, MemoryRouter, Routes, Route } from 'react-router-dom'

// Mock auth hook before importing components that use Navbar
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

import Collections from './Collections'
import api from '@/lib/api'
const mockedApi = api as unknown as typeof api & {
  get: ReturnType<typeof vi.fn>
  post: ReturnType<typeof vi.fn>
  delete: ReturnType<typeof vi.fn>
}

describe('Collections', () => {
  it('renders collections list', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, name: 'Digital Photos', type: 'digital', count: 100 },
        { id: 2, name: 'Scanned Photos', type: 'scanned', count: 50 }
      ]
    })

    render(
      <BrowserRouter>
        <Collections />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/Digital Photos/i)).toBeInTheDocument()
      expect(screen.getByText(/Scanned Photos/i)).toBeInTheDocument()
    })
  })

  it('loads folder contents when navigating to a folder', async () => {
    // Mock the API call for the folder contents
    mockedApi.get.mockResolvedValueOnce({
      data: {
        folder: { id: 1, name: '2017', path: '/unas/images/digital_photos/2017' },
        directories: [
          { id: 2, name: '20170625-FortBuenaVentura', path: '/unas/images/digital_photos/2017/20170625-FortBuenaVentura' }
        ],
        photos: [
          { id: 1, filename: 'photo1.jpg', url: '/api/photos/1/content' }
        ],
        breadcrumbs: []
      }
    })

    render(
      <MemoryRouter initialEntries={['/collections/1']}>
        <Routes>
          <Route path="/collections/:id" element={<Collections />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      // Check for the folder name in the heading
      expect(screen.getByRole('heading', { name: /2017/i })).toBeInTheDocument()
      // Check for the subdirectory
      expect(screen.getByText(/20170625-FortBuenaVentura/i)).toBeInTheDocument()
      // Check for the photo
      expect(screen.getByText(/photo1.jpg/i)).toBeInTheDocument()
    })
  })
})