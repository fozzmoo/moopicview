import { describe, it, expect, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BrowserRouter } from 'react-router-dom'

vi.mock('@/hooks/useAuth', () => ({
  useAuth: vi.fn(() => ({ user: { role: 'admin' }, isAuthenticated: true })),
}))

vi.mock('@/lib/api', () => ({
  default: {
    get: vi.fn(),
  },
}))

import TagsView from './TagsView'
import api from '@/lib/api'
const mockedApi = api as unknown as typeof api & {
  get: ReturnType<typeof vi.fn>
}

const mockTags = [
  { id: 1, name: 'Beach', photo_count: 12 },
  { id: 2, name: 'Mountain', photo_count: 5 },
  { id: 3, name: 'Forest', photo_count: 20 },
]

describe('TagsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('displays image count for each tag', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Beach')).toBeInTheDocument()
      expect(screen.getByText('(12 images)')).toBeInTheDocument()
      expect(screen.getByText('Mountain')).toBeInTheDocument()
      expect(screen.getByText('(5 images)')).toBeInTheDocument()
      expect(screen.getByText('Forest')).toBeInTheDocument()
      expect(screen.getByText('(20 images)')).toBeInTheDocument()
    })
  })

  it('displays singular "image" for tags with count of 1', async () => {
    mockedApi.get.mockResolvedValueOnce({
      data: [{ id: 1, name: 'Solo', photo_count: 1 }],
    })

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('(1 image)')).toBeInTheDocument()
    })
  })

  it('defaults to sorting by tag name', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      const tagNames = screen.getAllByRole('link').filter(el =>
        el.closest('[class*="grid"]')
      )
      expect(tagNames[0]).toHaveTextContent('Beach')
      expect(tagNames[1]).toHaveTextContent('Forest')
      expect(tagNames[2]).toHaveTextContent('Mountain')
    })
  })

  it('sorts by image count when clicking Sort by image count', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })
    const user = userEvent.setup()

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Beach')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Image count' }))

    const tagCards = screen.getAllByRole('link').filter(el =>
      el.closest('[class*="grid"]')
    )
    expect(tagCards[0]).toHaveTextContent('Forest')
    expect(tagCards[1]).toHaveTextContent('Beach')
    expect(tagCards[2]).toHaveTextContent('Mountain')
  })

  it('sorts by tag name when clicking Sort by tag name', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })
    const user = userEvent.setup()

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Beach')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Image count' }))
    await user.click(screen.getByRole('button', { name: 'Tag name' }))

    const tagCards = screen.getAllByRole('link').filter(el =>
      el.closest('[class*="grid"]')
    )
    expect(tagCards[0]).toHaveTextContent('Beach')
    expect(tagCards[1]).toHaveTextContent('Forest')
    expect(tagCards[2]).toHaveTextContent('Mountain')
  })

  it('shows search input', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search tags...')).toBeInTheDocument()
    })
  })

  it('filters tags by search query', async () => {
    mockedApi.get.mockResolvedValueOnce({ data: mockTags })
    const user = userEvent.setup()

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Beach')).toBeInTheDocument()
    })

    await user.type(screen.getByPlaceholderText('Search tags...'), 'Beach')

    expect(screen.getByText('Beach')).toBeInTheDocument()
    expect(screen.queryByText('Mountain')).not.toBeInTheDocument()
    expect(screen.queryByText('Forest')).not.toBeInTheDocument()
  })

  it('displays loading state', () => {
    mockedApi.get.mockReturnValueOnce(new Promise(() => {}))

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    expect(screen.getByText('Loading tags...')).toBeInTheDocument()
  })

  it('displays error state', async () => {
    mockedApi.get.mockRejectedValueOnce(new Error('Network error'))

    render(
      <BrowserRouter>
        <TagsView />
      </BrowserRouter>
    )

    await waitFor(() => {
      expect(screen.getByText('Failed to load tags')).toBeInTheDocument()
    })
  })
})
