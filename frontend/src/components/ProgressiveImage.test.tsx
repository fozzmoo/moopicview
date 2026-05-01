import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import ProgressiveImage from './ProgressiveImage';

describe('ProgressiveImage', () => {
  // Mock Image constructor
  class MockImage {
    src: string;
    onload: (() => void) | null;
    onerror: (() => void) | null;

    constructor() {
      this.src = '';
      this.onload = null;
      this.onerror = null;
    }
  }

  beforeEach(() => {
    vi.stubGlobal('Image', MockImage as any);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders with thumbnail and full image', () => {
    const { container } = render(
      <ProgressiveImage
        src="/api/photos/1/content"
        thumbnail="/thumbnails/1"
        alt="Test Photo"
        className="test-class"
      />
    );

    // Check that the component renders
    expect(container.firstChild).toBeTruthy();
  });

  it('shows placeholder initially', () => {
    render(
      <ProgressiveImage
        src="/api/photos/1/content"
        thumbnail="/thumbnails/1"
        alt="Test Photo"
      />
    );

    // Check that placeholder is shown
    const placeholder = screen.queryByRole('presentation');
    expect(placeholder).toBeTruthy();
  });

  it('uses correct thumbnail URL', () => {
    render(
      <ProgressiveImage
        src="/api/photos/1/content"
        thumbnail="/thumbnails/1"
        alt="Test Photo"
      />
    );

    // Check that thumbnail image element exists
    const images = screen.getAllByRole('img');
    const thumbnailImg = images.find(img => img.getAttribute('src') === '/thumbnails/1');
    expect(thumbnailImg).toBeTruthy();
  });

  it('uses correct full image URL', () => {
    render(
      <ProgressiveImage
        src="/api/photos/1/content"
        thumbnail="/thumbnails/1"
        alt="Test Photo"
      />
    );

    // Check that full image element exists
    const images = screen.getAllByRole('img');
    const fullImg = images.find(img => img.getAttribute('src') === '/api/photos/1/content');
    expect(fullImg).toBeTruthy();
  });

  it('applies custom className', () => {
    const { container } = render(
      <ProgressiveImage
        src="/api/photos/1/content"
        thumbnail="/thumbnails/1"
        alt="Test Photo"
        className="custom-class"
      />
    );

    // Check that custom className is applied
    expect(container.firstChild).toHaveClass('custom-class');
  });
});
