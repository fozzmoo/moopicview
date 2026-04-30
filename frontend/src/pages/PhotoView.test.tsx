import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

// Mock api module before importing PhotoView
vi.mock('@/lib/api', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
  },
}))

// Mock useAuth hook
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({
    user: { email: 'admin@example.com', role: 'admin' },
    isAuthenticated: true,
  }),
}))

import PhotoView from './PhotoView'

// Import after mocking
import api from '@/lib/api'
const mockedApi = api as unknown as typeof api & {
  get: ReturnType<typeof vi.fn>
  post: ReturnType<typeof vi.fn>
}

describe('PhotoView', () => {
  it('renders photo details', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: {
        photo: {
          id: 1,
          filename: 'test-photo.jpg',
          folder_id: 1,
          folder_name: 'Test Folder',
          photo_date: '2023-01-15',
          description: 'Test description',
          content_url: '/api/photos/1/content',
          collection: 'digital',
          prev_photo_id: null,
          next_photo_id: 2
        },
        breadcrumbs: [
          { id: 0, name: 'Collections', path: '' },
          { id: 1, name: 'Test Folder', path: '/test/path' }
        ]
      }
    })

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/test-photo.jpg/i)).toBeInTheDocument()
      expect(screen.getByText(/Test Folder/i)).toBeInTheDocument()
      expect(screen.getByText(/Test description/i)).toBeInTheDocument()
    })
  })

  it('displays loading state', () => {
    mockedApi.get.mockImplementation(() => new Promise(() => {}))

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('displays error message when photo not found', async () => {
    mockedApi.get.mockRejectedValueOnce({
      response: { status: 404 }
    })

    render(
      <MemoryRouter initialEntries={['/photo/999']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/photo not found/i)).toBeInTheDocument()
    })
  })

  it('handles download button click', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: {
        photo: {
          id: 1,
          filename: 'test-photo.jpg',
          folder_id: 1,
          folder_name: 'Test Folder',
          photo_date: '2023-01-15',
          description: 'Test description',
          content_url: '/api/photos/1/content',
          collection: 'digital',
          prev_photo_id: null,
          next_photo_id: 2
        },
        breadcrumbs: [
          { id: 0, name: 'Collections', path: '' },
          { id: 1, name: 'Test Folder', path: '/test/path' }
        ]
      }
    })

    // Mock fetch for download
    window.fetch = vi.fn().mockResolvedValueOnce({
      blob: vi.fn().mockResolvedValueOnce(new Blob(['test'])),
    })

    // Mock URL.createObjectURL
    const mockCreateObjectURL = vi.fn().mockReturnValue('blob:mock-url')
    window.URL.createObjectURL = mockCreateObjectURL

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/test-photo.jpg/i)).toBeInTheDocument()
    })

    const downloadButton = screen.getByRole('button', { name: /download/i })
    fireEvent.click(downloadButton)

    await waitFor(() => {
      expect(window.fetch).toHaveBeenCalledWith('/api/photos/1/content')
    })
  })

  it('displays navigation buttons for prev/next photos', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: {
        photo: {
          id: 1,
          filename: 'test-photo.jpg',
          folder_id: 1,
          folder_name: 'Test Folder',
          photo_date: '2023-01-15',
          description: 'Test description',
          content_url: '/api/photos/1/content',
          collection: 'digital',
          prev_photo_id: null,
          next_photo_id: 2
        },
        breadcrumbs: [
          { id: 0, name: 'Collections', path: '' },
          { id: 1, name: 'Test Folder', path: '/test/path' }
        ]
      }
    })

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/test-photo.jpg/i)).toBeInTheDocument()
    })

    // Next button should be visible - check for the link to photo 2
    const allLinks = screen.getAllByRole('link')
    const nextPhotoLink = allLinks.find(link => link.getAttribute('href') === '/photo/2')
    expect(nextPhotoLink).toBeInTheDocument()

    // Previous button should not be visible (prev_photo_id is null)
    // Check that there's no link to photo 0 (which would indicate a previous photo)
    const prevPhotoLink = allLinks.find(link => link.getAttribute('href') === '/photo/0')
    expect(prevPhotoLink).toBeUndefined()
  })
})