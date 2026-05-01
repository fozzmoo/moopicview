import { useState, useEffect } from 'react';

interface ProgressiveImageProps {
  src: string;
  thumbnail: string;
  alt?: string;
  className?: string;
}

export default function ProgressiveImage({ src, thumbnail, alt = '', className = '' }: ProgressiveImageProps) {
  const [isFullLoaded, setIsFullLoaded] = useState(false);
  const [isThumbnailLoaded, setIsThumbnailLoaded] = useState(false);

  useEffect(() => {
    // Reset state when src changes
    setIsFullLoaded(false);
    setIsThumbnailLoaded(false);

    // Preload full image
    const img = new Image();
    img.src = src;
    img.onload = () => setIsFullLoaded(true);
    img.onerror = () => {
      console.error(`Failed to load image: ${src}`);
      setIsFullLoaded(false);
    };
  }, [src]);

  return (
    <div className={`relative ${className}`}>
      {/* Thumbnail (always visible until full image loads) */}
      <img
        src={thumbnail}
        alt={alt}
        className={`absolute inset-0 w-full h-full object-cover transition-opacity duration-300 ${
          isFullLoaded ? 'opacity-0' : 'opacity-100'
        }`}
        onLoad={() => setIsThumbnailLoaded(true)}
        onError={(e) => {
          console.error(`Failed to load thumbnail: ${thumbnail}`);
          (e.target as HTMLImageElement).style.display = 'none';
        }}
      />
      
      {/* Full image (fades in when loaded) */}
      <img
        src={src}
        alt={alt}
        className={`w-full h-full object-cover transition-opacity duration-300 ${
          isFullLoaded ? 'opacity-100' : 'opacity-0'
        }`}
      />

      {/* Loading placeholder (shown while thumbnail is loading) */}
      {!isThumbnailLoaded && !isFullLoaded && (
        <div className="absolute inset-0 bg-gray-200 animate-pulse" role="presentation" />
      )}
    </div>
  );
}
