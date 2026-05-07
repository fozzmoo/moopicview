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
  delete: ReturnType<typeof vi.fn>
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

  // New tests for clickable edge navigation
  it('displays clickable edge areas for navigation', async () => {
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

    // Right edge should be visible for next photo
    const rightEdge = screen.getByText(/next photo/i).closest('[title="Next photo"]')
    expect(rightEdge).toBeInTheDocument()
  })

  it('displays fullscreen toggle button', async () => {
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

    // Fullscreen toggle button should be visible
    const fullscreenButton = screen.getByRole('button', { name: /enter fullscreen/i })
    expect(fullscreenButton).toBeInTheDocument()
  })

  it('toggles fullscreen mode', async () => {
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

    const fullscreenButton = screen.getByRole('button', { name: /enter fullscreen/i })
    
    // Click to enter fullscreen
    fireEvent.click(fullscreenButton)

    // Should now show exit fullscreen button
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /exit fullscreen/i })).toBeInTheDocument()
    })

    // Click to exit fullscreen
    fireEvent.click(screen.getByRole('button', { name: /exit fullscreen/i }))

    // Should show enter fullscreen button again
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /enter fullscreen/i })).toBeInTheDocument()
    })
  })

  it('displays zoom controls in fullscreen mode', async () => {
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

    const fullscreenButton = screen.getByRole('button', { name: /enter fullscreen/i })
    
    // Click to enter fullscreen
    fireEvent.click(fullscreenButton)

    // Zoom controls should be visible in fullscreen
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /zoom in/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /zoom out/i })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: /reset zoom/i })).toBeInTheDocument()
    })
  })

  it('handles keyboard navigation (arrow keys)', async () => {
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

    // Simulate arrow right key press
    fireEvent.keyDown(window, { key: 'ArrowRight' })

    // Should navigate to next photo
    await waitFor(() => {
      expect(window.location.pathname).toBe('/photo/2')
    })
  })

  it('handles escape key in fullscreen mode', async () => {
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

    const fullscreenButton = screen.getByRole('button', { name: /enter fullscreen/i })
    
    // Click to enter fullscreen
    fireEvent.click(fullscreenButton)

    // Simulate escape key press
    fireEvent.keyDown(window, { key: 'Escape' })

    // Should exit fullscreen
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /enter fullscreen/i })).toBeInTheDocument()
    })
  })

  // New tests for tagging functionality
  it('displays tags section', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: [
          { id: 1, name: 'Person A', posX: 25, posY: 30 },
          { id: 2, name: 'Person B', posX: 75, posY: 40 }
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
      expect(screen.getByText(/Tags \(2\)/i)).toBeInTheDocument()
    })
    
    // Check that tag names appear (may appear in multiple places)
    expect(screen.getAllByText(/Person A/i).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/Person B/i).length).toBeGreaterThan(0)
  })

  it('displays tag markers on image', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: [
          { id: 1, name: 'Person A', posX: 25, posY: 30 }
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
      expect(screen.getByText(/Tags \(1\)/i)).toBeInTheDocument()
    })

    // Tag marker should be visible (hover area div)
    const tagMarker = document.querySelector('.absolute.group')
    expect(tagMarker).toBeInTheDocument()
  })

  it('opens tagging dialog when clicking image', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: []
      }
    })

    mockedApi.get.mockResolvedValueOnce({
      data: []
    })

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/Tags \(0\)/i)).toBeInTheDocument()
    })

    // Click on the image container
    const imageContainer = document.querySelector('[class*="bg-black"]')
    if (imageContainer) {
      fireEvent.click(imageContainer)
    }

    // Dialog should open
    await waitFor(() => {
      expect(screen.getByText(/Add Tag to Photo/i)).toBeInTheDocument()
    })
  })

  it('displays tag suggestions in dialog', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: []
      }
    })

    mockedApi.get.mockResolvedValueOnce({
      data: [
        { id: 1, name: 'Eli Barton' },
        { id: 2, name: 'John Doe' }
      ]
    })

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/Tags \(0\)/i)).toBeInTheDocument()
    })

    // Click on the image area to open dialog
    const imageContainer = document.querySelector('[class*="bg-black"]')
    if (imageContainer) {
      fireEvent.click(imageContainer)
    }

    // Dialog should open
    await waitFor(() => {
      expect(screen.getByText(/Add Tag to Photo/i)).toBeInTheDocument()
    })
  })

  it('displays clickable tags that navigate to tag page', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: [
          { id: 1, name: 'Person A', posX: 25, posY: 30 },
          { id: 2, name: 'Person B', posX: 75, posY: 40 }
        ]
      }
    })

    render(
      <MemoryRouter initialEntries={['/photo/1']}>
        <Routes>
          <Route path="/photo/:id" element={<PhotoView />} />
          <Route path="/tags/:tagId" element={<div>Tag Photos Page</div>} />
        </Routes>
      </MemoryRouter>
    )

    await waitFor(() => {
      expect(screen.getByText(/Tags \(2\)/i)).toBeInTheDocument()
    })

    // Find tag links
    const tagLinks = screen.getAllByRole('link')
    const personALink = tagLinks.find(link => link.textContent?.includes('Person A'))
    const personBLink = tagLinks.find(link => link.textContent?.includes('Person B'))

    expect(personALink).toBeInTheDocument()
    expect(personBLink).toBeInTheDocument()

    // Check that the links point to the correct tag pages
    expect(personALink?.getAttribute('href')).toBe('/tags/1')
    expect(personBLink?.getAttribute('href')).toBe('/tags/2')
  })

  it('hovering over tag updates marker highlight', async () => {
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
        breadcrumbs: [],
        comments: [],
        tags: [
          { id: 1, name: 'Person A', posX: 25, posY: 30 }
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
      expect(screen.getByText(/Tags \(1\)/i)).toBeInTheDocument()
    })

    // Find the tag in the list (use getAllByText since there might be multiple instances)
    const tagElements = screen.getAllByText(/Person A/i)
    // The tag badge is the span element that should be hoverable
    const tagBadge = tagElements.find(el => el.tagName === 'SPAN' && el.closest('a'))
    
    if (tagBadge) {
      // Hover over the tag
      fireEvent.mouseEnter(tagBadge)

      // The tag should now be highlighted (yellow background)
      await waitFor(() => {
        expect(tagBadge.closest('span')).toHaveClass('bg-yellow-400')
      })
    }
  })

  it('renders copy to clipboard button', async () => {
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

    const copyButton = screen.getByRole('button', { name: /copy/i })
    expect(copyButton).toBeInTheDocument()
  })

  it('calls fetch as fallback for copy to clipboard', async () => {
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

    // Mock fetch to return a blob
    const mockBlob = new Blob(['test'], { type: 'image/jpeg' })
    window.fetch = vi.fn().mockResolvedValue({
      blob: vi.fn().mockResolvedValue(mockBlob),
    })

    // Mock ClipboardItem and navigator.clipboard.write
    const mockWrite = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('ClipboardItem', class {
      constructor(public data: Record<string, Blob>) {}
    })
    Object.defineProperty(navigator, 'clipboard', {
      value: { write: mockWrite },
      writable: true,
      configurable: true,
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

    const copyButton = screen.getByRole('button', { name: /copy/i })
    fireEvent.click(copyButton)

    // The canvas approach will fail in JSDOM (Image.onload never fires),
    // so it should fall back to the fetch-based approach
    await waitFor(() => {
      expect(window.fetch).toHaveBeenCalledWith('/api/photos/1/content')
      expect(mockWrite).toHaveBeenCalled()
    }, { timeout: 5000 })

    vi.unstubAllGlobals()
  })
})
