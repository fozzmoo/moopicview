import { useState, useEffect, useRef } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import api from '@/lib/api';
import { Calendar, Folder as FolderIcon, MapPin, Download, Copy, ChevronLeft, ChevronRight, Edit2, Tag, Maximize2, Minimize2, ZoomIn, ZoomOut, RotateCcw, Plus, X } from 'lucide-react';
import { Navbar } from '../components/navbar';
import { Card, CardContent } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Input } from '@/components/ui/input';
import { formatDate } from '@/lib/dateUtils';
import { useAuth } from '@/hooks/useAuth';
import ProgressiveImage from '@/components/ProgressiveImage';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

export default function PhotoView() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const [photo, setPhoto] = useState<any>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<any[]>([]);
  const [comments, setComments] = useState<any[]>([]);
  const [tags, setTags] = useState<any[]>([]);
  const [fileInfo, setFileInfo] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [isEditingDate, setIsEditingDate] = useState(false);
  const [editDate, setEditDate] = useState('');
  const [dateError, setDateError] = useState('');
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const [editDescription, setEditDescription] = useState('');
  const [newComment, setNewComment] = useState('');
  const [commentError, setCommentError] = useState('');
  const [isSubmittingComment, setIsSubmittingComment] = useState(false);
  const [showTags, setShowTags] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [zoomLevel, setZoomLevel] = useState(1);
  const [panOffset, setPanOffset] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [hoveredTagId, setHoveredTagId] = useState<number | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const photoRes = await api.get(`/api/photos/${id}`);
        setPhoto(photoRes.data.photo);
        setBreadcrumbs(photoRes.data.breadcrumbs || []);
        setComments(photoRes.data.comments || []);
        setTags(photoRes.data.tags || []);
        setFileInfo(photoRes.data.file_info || '');
      } catch (err) {
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [id]);

  const handleDownload = async () => {
    try {
      const response = await fetch(photo.content_url);
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = photo.filename;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Download failed:', error);
      alert('Failed to download image');
    }
  };

  const handleCopyToClipboard = async () => {
    try {
      const img = new Image();
      img.crossOrigin = 'anonymous';
      img.src = photo.content_url;
      await new Promise<void>((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error('Image load timeout')), 3000);
        img.onload = () => { clearTimeout(timeout); resolve(); };
        img.onerror = () => { clearTimeout(timeout); reject(new Error('Image load failed')); };
      });
      const canvas = document.createElement('canvas');
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;
      const ctx = canvas.getContext('2d')!;
      ctx.drawImage(img, 0, 0);
      const blob = await new Promise<Blob>((resolve) => canvas.toBlob((b) => resolve(b!), 'image/png'));
      await navigator.clipboard.write([
        new ClipboardItem({ 'image/png': blob })
      ]);
    } catch (error) {
      // Fallback: try fetch-based approach
      try {
        const response = await fetch(photo.content_url);
        const blob = await response.blob();
        const type = blob.type || 'image/jpeg';
        const item = new ClipboardItem({ [type]: blob });
        await navigator.clipboard.write([item]);
      } catch (fallbackError) {
        console.error('Copy failed:', error);
        alert('Failed to copy image to clipboard');
      }
    }
  };

  // Tagging dialog state
  const [isTagDialogOpen, setIsTagDialogOpen] = useState(false);
  const [tagPosition, setTagPosition] = useState({ x: 50, y: 50 });
  const [newTagName, setNewTagName] = useState('');
  const [existingTags, setExistingTags] = useState<any[]>([]);
  const [tagSuggestions, setTagSuggestions] = useState<any[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [isSubmittingTag, setIsSubmittingTag] = useState(false);
  const [tagError, setTagError] = useState('');
  const tagNameInputRef = useRef<HTMLInputElement>(null);

  // Focus tag name input when dialog opens
  useEffect(() => {
    if (isTagDialogOpen && tagNameInputRef.current) {
      setTimeout(() => {
        tagNameInputRef.current?.focus();
      }, 100);
    }
  }, [isTagDialogOpen]);

  // Fetch existing tags for autocomplete
  useEffect(() => {
    const fetchTags = async () => {
      try {
        const tagsRes = await api.get('/api/tags');
        setExistingTags(tagsRes.data);
      } catch (err) {
        console.error('Failed to fetch tags:', err);
      }
    };
    fetchTags();
  }, []);

  // Filter tags based on input for suggestions
  useEffect(() => {
    if (newTagName.trim().length > 0) {
      const filtered = existingTags.filter(tag => 
        tag.name.toLowerCase().includes(newTagName.toLowerCase())
      );
      setTagSuggestions(filtered.slice(0, 5)); // Show max 5 suggestions
      setShowSuggestions(true);
    } else {
      setTagSuggestions([]);
      setShowSuggestions(false);
    }
  }, [newTagName, existingTags]);

  // Handle tag submission
  const handleAddTag = async () => {
    if (!newTagName.trim()) {
      setTagError('Tag name is required');
      return;
    }

    setIsSubmittingTag(true);
    setTagError('');

    try {
      const response = await api.post(`/api/photos/${id}/tags`, {
        tagName: newTagName.trim(),
        posX: tagPosition.x,
        posY: tagPosition.y
      });
      
      // Add the new tag to the list
      setTags([...tags, response.data]);
      setIsTagDialogOpen(false);
      setNewTagName('');
    } catch (err: any) {
      console.error('Failed to add tag:', err);
      setTagError(err.response?.data || 'Failed to add tag');
    } finally {
      setIsSubmittingTag(false);
    }
  };

  // Handle tag deletion
  const handleDeleteTag = async (tagId: number) => {
    try {
      await api.delete(`/api/photos/${id}/tags/${tagId}`);
      // Remove tag from local state
      setTags(tags.filter(tag => tag.id !== tagId));
    } catch (err) {
      console.error('Failed to delete tag:', err);
      alert('Failed to delete tag');
    }
  };

  // Fullscreen functions
  const enterFullscreen = () => {
    setIsFullscreen(true);
    setZoomLevel(1);
    setPanOffset({ x: 0, y: 0 });
    
    const elem = document.documentElement;
    if (elem.requestFullscreen) {
      elem.requestFullscreen();
    } else if ((elem as any).webkitRequestFullscreen) {
      (elem as any).webkitRequestFullscreen();
    } else if ((elem as any).msRequestFullscreen) {
      (elem as any).msRequestFullscreen();
    }
  };

  const exitFullscreen = () => {
    setIsFullscreen(false);
    setZoomLevel(1);
    setPanOffset({ x: 0, y: 0 });
    
    // Exit browser fullscreen
    if (document.exitFullscreen) {
      document.exitFullscreen();
    } else if ((document as any).webkitExitFullscreen) {
      (document as any).webkitExitFullscreen();
    } else if ((document as any).msExitFullscreen) {
      (document as any).msExitFullscreen();
    }
  };

  const toggleFullscreen = () => {
    if (isFullscreen) {
      exitFullscreen();
    } else {
      enterFullscreen();
    }
  };

  // Zoom functions
  const handleZoomIn = () => {
    setZoomLevel(prev => Math.min(prev + 0.25, 3));
  };

  const handleZoomOut = () => {
    setZoomLevel(prev => Math.max(prev - 0.25, 0.5));
  };

  const handleResetZoom = () => {
    setZoomLevel(1);
    setPanOffset({ x: 0, y: 0 });
  };

  // Pan functions
  const handleMouseDown = (e: React.MouseEvent) => {
    if (zoomLevel > 1) {
      setIsDragging(true);
      setDragStart({ x: e.clientX - panOffset.x, y: e.clientY - panOffset.y });
    }
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    if (isDragging && zoomLevel > 1) {
      setPanOffset({
        x: e.clientX - dragStart.x,
        y: e.clientY - dragStart.y
      });
    }
  };

  const handleMouseUp = () => {
    setIsDragging(false);
  };

  const handleMouseLeave = () => {
    setIsDragging(false);
  };

  // Handle image click to add tag
  const handleImageClick = (e: React.MouseEvent) => {
    if (!user || isFullscreen) return; // Don't add tags in fullscreen mode or if not logged in
    
    // Get the container that holds the image (the relative positioned div)
    const container = e.currentTarget.parentElement;
    if (!container) return;
    
    const containerRect = container.getBoundingClientRect();
    
    // Calculate position relative to the container
    const x = ((e.clientX - containerRect.left) / containerRect.width) * 100;
    const y = ((e.clientY - containerRect.top) / containerRect.height) * 100;
    
    setTagPosition({ x, y });
    setNewTagName('');
    setTagError('');
    setIsTagDialogOpen(true);
  };

  // Sync fullscreen state with browser's fullscreen API
  useEffect(() => {
    const handleFullscreenChange = () => {
      const isCurrentlyFullscreen = !!document.fullscreenElement || 
        !!(document as any).webkitFullscreenElement || 
        !!(document as any).msFullscreenElement;
      
      setIsFullscreen(isCurrentlyFullscreen);
      if (!isCurrentlyFullscreen) {
        setZoomLevel(1);
        setPanOffset({ x: 0, y: 0 });
      }
    };

    document.addEventListener('fullscreenchange', handleFullscreenChange);
    document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
    document.addEventListener('MSFullscreenChange', handleFullscreenChange);

    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      document.removeEventListener('webkitfullscreenchange', handleFullscreenChange);
      document.removeEventListener('MSFullscreenChange', handleFullscreenChange);
    };
  }, []);

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (loading || !photo) return;
      
      if (e.key === 'ArrowLeft' && photo.prev_photo_id) {
        navigate(`/photo/${photo.prev_photo_id}`);
      } else if (e.key === 'ArrowRight' && photo.next_photo_id) {
        navigate(`/photo/${photo.next_photo_id}`);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [loading, photo, navigate]);

  // Update edit date when photo changes
  useEffect(() => {
    if (photo) {
      // Convert database date format to input format
      // Database stores as YYYY-MM-DD, but we want to show YYYY, YYYY-MM, or YYYY-MM-DD based on precision
      if (photo.photo_date && photo.date_precision) {
        const parts = photo.photo_date.split('-');
        if (photo.date_precision === 'year') {
          setEditDate(parts[0]);
        } else if (photo.date_precision === 'month') {
          setEditDate(`${parts[0]}-${parts[1]}`);
        } else {
          setEditDate(photo.photo_date);
        }
      } else {
        setEditDate('');
      }
      // Update edit description when photo changes
      setEditDescription(photo.description || '');
    }
  }, [photo]);

  // Check if user is admin
  const isAdmin = user?.role === 'admin';

  const parseAndValidateDate = (dateStr: string): { date: string; precision: string } | null => {
    if (!dateStr || dateStr.trim() === '') {
      return { date: '', precision: 'unknown' };
    }

    const trimmed = dateStr.trim();
    
    // Try YYYY-MM-DD format
    const exactMatch = trimmed.match(/^(\d{4})-(\d{2})-(\d{2})$/);
    if (exactMatch) {
      const year = parseInt(exactMatch[1]);
      const month = parseInt(exactMatch[2]);
      const day = parseInt(exactMatch[3]);
      if (year >= 1900 && year <= 2100 && month >= 1 && month <= 12 && day >= 1 && day <= 31) {
        return { date: trimmed, precision: 'exact' };
      }
    }

    // Try YYYY-MM format
    const monthMatch = trimmed.match(/^(\d{4})-(\d{2})$/);
    if (monthMatch) {
      const year = parseInt(monthMatch[1]);
      const month = parseInt(monthMatch[2]);
      if (year >= 1900 && year <= 2100 && month >= 1 && month <= 12) {
        return { date: `${trimmed}-01`, precision: 'month' };
      }
    }

    // Try YYYY format
    const yearMatch = trimmed.match(/^(\d{4})$/);
    if (yearMatch) {
      const year = parseInt(yearMatch[1]);
      if (year >= 1900 && year <= 2100) {
        return { date: `${trimmed}-01-01`, precision: 'year' };
      }
    }

    return null;
  };

  const updatePhotoDate = async () => {
    const parsed = parseAndValidateDate(editDate);
    
    if (!parsed) {
      setDateError('Invalid date format. Use YYYY, YYYY-MM, or YYYY-MM-DD');
      return;
    }

    try {
      await api.post(`/api/admin/photos/${id}/date`, {
        photo_date: parsed.date || null,
        date_precision: parsed.precision
      });
      // Refresh photo data
      const photoRes = await api.get(`/api/photos/${id}`);
      setPhoto(photoRes.data.photo);
      setBreadcrumbs(photoRes.data.breadcrumbs || []);
      setIsEditingDate(false);
      setDateError('');
    } catch (err) {
      console.error('Failed to update photo date:', err);
      setDateError('Failed to update photo date');
    }
  };

  const updatePhotoDescription = async () => {
    try {
      await api.post(`/api/admin/photos/${id}/description`, {
        description: editDescription
      });
      // Refresh photo data
      const photoRes = await api.get(`/api/photos/${id}`);
      setPhoto(photoRes.data.photo);
      setBreadcrumbs(photoRes.data.breadcrumbs || []);
      setIsEditingDescription(false);
    } catch (err) {
      console.error('Failed to update photo description:', err);
      alert('Failed to update photo description');
    }
  };

  const postComment = async () => {
    if (!newComment.trim()) {
      setCommentError('Comment cannot be empty');
      return;
    }

    setIsSubmittingComment(true);
    setCommentError('');

    try {
      const response = await api.post(`/api/photos/${id}/comments`, {
        content: newComment
      });
      
      // Add the new comment to the list
      setComments([...comments, response.data]);
      setNewComment('');
    } catch (err: any) {
      console.error('Failed to post comment:', err);
      setCommentError(err.response?.data || 'Failed to post comment');
    } finally {
      setIsSubmittingComment(false);
    }
  };

  if (loading) return <div className="min-h-screen bg-background flex items-center justify-center">Loading...</div>;
  if (!photo) return <div className="min-h-screen bg-background flex items-center justify-center">Photo not found</div>;

  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <div className="container mx-auto px-4 py-6">
        <nav className="flex items-center gap-2 text-sm text-muted-foreground mb-6">
          {breadcrumbs.map((crumb, index) => (
            <span key={crumb.id || index}>
              {index > 0 && <span className="text-muted-foreground/50">/</span>}
              {crumb.id === 0 ? (
                <Link to={`/collections`} className="hover:text-foreground">
                  {crumb.name}
                </Link>
              ) : crumb.id > 0 ? (
                <Link to={`/collections/${crumb.id}`} className="hover:text-foreground">
                  {crumb.name}
                </Link>
              ) : (
                <span className="text-foreground font-medium">{crumb.name}</span>
              )}
            </span>
          ))}
          {photo && (
            <span className="text-muted-foreground ml-1">
              ({photo.collection === 'digital' ? 'Digital' : 'Scanned'})
            </span>
          )}
        </nav>

        <div className="flex items-start gap-4 lg:gap-8">
          <div className="flex-1 flex flex-col gap-6">
              <div className="flex-1">
                {isFullscreen ? (
                  <div 
                    className="relative bg-black w-full h-full fixed inset-0 z-50"
                    style={{
                      cursor: zoomLevel > 1 ? (isDragging ? 'grabbing' : 'grab') : 'default'
                    }}
                    onMouseDown={handleMouseDown}
                    onMouseMove={handleMouseMove}
                    onMouseUp={handleMouseUp}
                    onMouseLeave={handleMouseLeave}
                  >
                    <ProgressiveImage
                      src={photo.content_url}
                      thumbnail={`/thumbnails/${id}`}
                      alt={photo.filename}
                      className="h-screen w-full"
                      style={{
                        transform: `scale(${zoomLevel}) translate(${panOffset.x / zoomLevel}px, ${panOffset.y / zoomLevel}px)`,
                        cursor: zoomLevel > 1 ? (isDragging ? 'grabbing' : 'grab') : 'default'
                      }}
                      fit="cover"
                    />
                    {/* Edge Click Navigation - Left 15% */}
                    {photo.prev_photo_id && (
                      <Link
                        to={`/photo/${photo.prev_photo_id}`}
                        className="absolute left-0 top-0 h-full w-1/6 cursor-pointer z-20 opacity-0 hover:opacity-50 transition-opacity bg-black/50"
                        title="Previous photo"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
                          <ChevronLeft className="h-8 w-8 text-white" />
                        </div>
                      </Link>
                    )}

                    {/* Edge Click Navigation - Right 15% */}
                    {photo.next_photo_id && (
                      <Link
                        to={`/photo/${photo.next_photo_id}`}
                        className="absolute right-0 top-0 h-full w-1/6 cursor-pointer z-20 opacity-0 hover:opacity-50 transition-opacity bg-black/50"
                        title="Next photo"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <div className="absolute right-1/2 top-1/2 translate-x-1/2 -translate-y-1/2">
                          <ChevronRight className="h-8 w-8 text-white" />
                        </div>
                      </Link>
                    )}
                    
                    {/* Controls Container */}
                    <div className="absolute top-2 right-2 z-30 flex gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 bg-black/50 hover:bg-black/70 text-white"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleZoomIn();
                        }}
                        title="Zoom In"
                      >
                        <ZoomIn className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 bg-black/50 hover:bg-black/70 text-white"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleZoomOut();
                        }}
                        title="Zoom Out"
                      >
                        <ZoomOut className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 bg-black/50 hover:bg-black/70 text-white"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleResetZoom();
                        }}
                        title="Reset Zoom"
                      >
                        <RotateCcw className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 w-8 p-0 bg-black/50 hover:bg-black/70 text-white"
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleFullscreen();
                        }}
                        title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
                      >
                        {isFullscreen ? (
                          <Minimize2 className="h-4 w-4" />
                        ) : (
                          <Maximize2 className="h-4 w-4" />
                        )}
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className={`h-8 w-8 p-0 ${showTags ? 'bg-primary text-white' : 'bg-black/50 hover:bg-black/70 text-white'}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          setShowTags(!showTags);
                        }}
                        title={showTags ? 'Hide Tags' : 'Show Tags'}
                      >
                        <Tag className="h-4 w-4" />
                      </Button>
                    </div>
                    
                    {/* Tag Markers Overlay */}
                    {tags.map((tag) => (
                      <div
                        key={tag.id}
                        className={`absolute group ${hoveredTagId === tag.id ? 'z-40' : ''}`}
                        style={{
                          left: `${tag.posX}%`,
                          top: `${tag.posY}%`,
                          transform: 'translate(-50%, -50%)',
                          opacity: showTags || hoveredTagId === tag.id ? 1 : 0,
                          pointerEvents: showTags || hoveredTagId === tag.id ? 'auto' : 'none'
                        }}
                      >
                        <div className="w-16 h-16 -ml-8 -mt-8 cursor-pointer" />
                        <div className={`absolute w-3 h-3 rounded-full transform -translate-x-1/2 -translate-y-1/2 transition-all duration-200 ${
                          hoveredTagId === tag.id 
                            ? 'bg-yellow-400 scale-150 shadow-lg shadow-yellow-400/50' 
                            : 'bg-primary'
                        }`} />
                        <div className={`absolute left-1/2 top-full mt-1 px-2 py-1 bg-black/70 text-white text-xs rounded whitespace-nowrap pointer-events-none -translate-x-1/2 transition-opacity duration-200 ${
                          hoveredTagId === tag.id ? 'opacity-100' : 'opacity-70'
                        }`}>
                          {tag.name}
                        </div>
                        {hoveredTagId === tag.id && user && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleDeleteTag(tag.id);
                            }}
                            className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity hover:bg-red-600 z-50"
                            title="Delete tag"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <Card className="overflow-hidden flex-1">
                    <div className="relative bg-black/5 w-full h-full flex items-center justify-center">
                      {/* Edge Click Navigation - Left 15% */}
                      {photo.prev_photo_id && (
                        <Link
                          to={`/photo/${photo.prev_photo_id}`}
                          className="absolute left-0 top-0 h-full w-1/6 cursor-pointer z-20 opacity-0 hover:opacity-50 transition-opacity bg-black/50"
                          title="Previous photo"
                        >
                          <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2">
                            <ChevronLeft className="h-8 w-8 text-white" />
                          </div>
                        </Link>
                      )}

                      {/* Edge Click Navigation - Right 15% */}
                      {photo.next_photo_id && (
                        <Link
                          to={`/photo/${photo.next_photo_id}`}
                          className="absolute right-0 top-0 h-full w-1/6 cursor-pointer z-20 opacity-0 hover:opacity-50 transition-opacity bg-black/50"
                          title="Next photo"
                        >
                          <div className="absolute right-1/2 top-1/2 translate-x-1/2 -translate-y-1/2">
                            <ChevronRight className="h-8 w-8 text-white" />
                          </div>
                        </Link>
                      )}
                    
                      {/* Controls Container */}
                      <div className="absolute top-2 right-2 z-20 flex gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-8 w-8 p-0 bg-black/50 hover:bg-black/70 text-white"
                          onClick={(e) => {
                            e.stopPropagation();
                            toggleFullscreen();
                          }}
                          title={isFullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
                        >
                          {isFullscreen ? (
                            <Minimize2 className="h-4 w-4" />
                          ) : (
                            <Maximize2 className="h-4 w-4" />
                          )}
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className={`h-8 w-8 p-0 ${showTags ? 'bg-primary text-white' : 'bg-black/50 hover:bg-black/70 text-white'}`}
                          onClick={(e) => {
                            e.stopPropagation();
                            setShowTags(!showTags);
                          }}
                          title={showTags ? 'Hide Tags' : 'Show Tags'}
                        >
                          <Tag className="h-4 w-4" />
                        </Button>
                      </div>
                      
                      <ProgressiveImage
                        src={photo.content_url}
                        thumbnail={`/thumbnails/${id}`}
                        alt={photo.filename}
                        className="h-full w-full rounded-lg transition-transform"
                        style={{}}
                        fit="contain"
                      />
                      
                      {/* Click area for adding tags (behind navigation and controls) */}
                      {user && (
                        <div 
                          className="absolute inset-0 z-5 cursor-crosshair"
                          onClick={handleImageClick}
                          title="Click to add a tag"
                        />
                      )}
                      
                      {/* Tag Markers Overlay */}
                      {tags.map((tag) => (
                        <div
                          key={tag.id}
                          className={`absolute group ${hoveredTagId === tag.id ? 'z-40' : ''}`}
                          style={{
                            left: `${tag.posX}%`,
                            top: `${tag.posY}%`,
                            transform: 'translate(-50%, -50%)',
                            opacity: showTags || hoveredTagId === tag.id ? 1 : 0,
                            pointerEvents: showTags || hoveredTagId === tag.id ? 'auto' : 'none'
                          }}
                        >
                          <div className="w-16 h-16 -ml-8 -mt-8 cursor-pointer" />
                          <div className={`absolute w-3 h-3 rounded-full transform -translate-x-1/2 -translate-y-1/2 transition-all duration-200 ${
                            hoveredTagId === tag.id 
                              ? 'bg-yellow-400 scale-150 shadow-lg shadow-yellow-400/50' 
                              : 'bg-primary'
                          }`} />
                          <div className={`absolute left-1/2 top-full mt-1 px-2 py-1 bg-black/70 text-white text-xs rounded whitespace-nowrap pointer-events-none -translate-x-1/2 transition-opacity duration-200 ${
                            hoveredTagId === tag.id ? 'opacity-100' : 'opacity-70'
                          }`}>
                            {tag.name}
                          </div>
                          {hoveredTagId === tag.id && user && (
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                handleDeleteTag(tag.id);
                              }}
                              className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity hover:bg-red-600 z-50"
                              title="Delete tag"
                            >
                              <X className="w-3 h-3" />
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  </Card>
                )}
              </div>

            <div className="w-full space-y-4">
              <Card>
                <CardContent className="p-6 space-y-4">
                  <div>
                    <h1 className="text-xl font-bold text-foreground mb-2">{photo.filename}</h1>
                  </div>

                  <div className="space-y-3 pt-4 border-t">
                    <div className="flex items-center gap-3 text-sm">
                      <FolderIcon className="h-4 w-4 text-muted-foreground" />
                      <span className="font-medium capitalize text-foreground">{photo.collection}</span>
                    </div>
                    <div className="flex items-start gap-3 text-sm">
                      <div className="h-4 w-4 text-muted-foreground mt-0.5 flex-shrink-0">
                        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="lucide lucide-align-left"><line x1="21" x2="3" y1="6" y2="6"/><line x1="15" x2="3" y1="12" y2="12"/><line x1="17" x2="3" y1="18" y2="18"/></svg>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <p className="text-foreground break-words">
                            {photo.description || 'No description'}
                          </p>
                          {isAdmin && (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0 opacity-50 hover:opacity-100 transition-opacity flex-shrink-0"
                              onClick={() => setIsEditingDescription(true)}
                            >
                              <Edit2 className="h-3 w-3" />
                            </Button>
                          )}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 text-sm">
                      <Calendar className="h-4 w-4 text-muted-foreground" />
                      <div className="flex items-center gap-2 flex-1">
                        <span className="text-foreground">
                          {formatDate(photo.photo_date, photo.date_precision)}
                        </span>
                        {isAdmin && (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-6 w-6 p-0 opacity-50 hover:opacity-100 transition-opacity"
                            onClick={() => setIsEditingDate(true)}
                          >
                            <Edit2 className="h-3 w-3" />
                          </Button>
                        )}
                      </div>
                    </div>
                    {fileInfo && (
                      <div className="flex items-center gap-3 text-sm">
                        <div className="h-4 w-4 text-muted-foreground flex-shrink-0">
                          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="lucide lucide-info"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>
                        </div>
                        <span className="text-muted-foreground">{fileInfo}</span>
                      </div>
                    )}
                    <div className="flex items-center gap-3 text-sm">
                      <MapPin className="h-4 w-4 text-muted-foreground" />
                      <span className="text-muted-foreground">Location not set</span>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardContent className="p-6">
                  <h3 className="font-semibold mb-3 text-sm text-foreground">Actions</h3>
                  <div className="space-y-2">
                    <Button variant="outline" className="w-full justify-start" onClick={handleDownload}>
                      <Download className="mr-2 h-4 w-4" />
                      Download
                    </Button>
                    <Button variant="outline" className="w-full justify-start" onClick={handleCopyToClipboard}>
                      <Copy className="mr-2 h-4 w-4" />
                      Copy
                    </Button>
                  </div>
                 </CardContent>
               </Card>

               {/* Tags Section */}
               <Card>
                 <CardContent className="p-6">
                   <div className="flex items-center justify-between mb-4">
                     <h3 className="font-semibold text-sm text-foreground">Tags ({tags.length})</h3>
                     {user && (
                       <Button 
                         variant="outline" 
                         size="sm"
                         onClick={() => {
                           setTagPosition({ x: 50, y: 50 });
                           setNewTagName('');
                           setTagError('');
                           setIsTagDialogOpen(true);
                         }}
                       >
                         <Plus className="h-4 w-4 mr-1" />
                         Add Tag
                       </Button>
                     )}
                   </div>
                   
                    {/* Tags List */}
                    <div className="flex flex-wrap gap-2 mb-4">
                      {tags.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No tags yet</p>
                      ) : (
                        tags.map((tag) => (
                          <Link 
                            to={`/tags/${tag.id}`}
                            key={tag.id}
                          >
                            <span 
                              className={`inline-flex items-center gap-1 px-3 py-1 rounded-full text-sm cursor-pointer transition-all duration-200 ${
                                hoveredTagId === tag.id 
                                  ? 'bg-yellow-400 text-black scale-105 shadow-md' 
                                  : 'bg-primary/10 text-primary hover:bg-primary/20'
                              }`}
                              onMouseEnter={() => setHoveredTagId(tag.id)}
                              onMouseLeave={() => setHoveredTagId(null)}
                            >
                              {tag.name}
                              {user && (
                                <button
                                  onClick={(e) => {
                                    e.preventDefault();
                                    e.stopPropagation();
                                    handleDeleteTag(tag.id);
                                  }}
                                  className="ml-1 p-0.5 hover:bg-red-500 hover:text-white rounded transition-colors"
                                  title="Delete tag"
                                >
                                  <X className="w-3 h-3" />
                                </button>
                              )}
                            </span>
                          </Link>
                        ))
                      )}
                    </div>
                 </CardContent>
               </Card>

               {/* Comments Section */}
              <Card>
                <CardContent className="p-6">
                  <h3 className="font-semibold mb-4 text-sm text-foreground">Comments ({comments.length})</h3>
                  
                  {/* Comments List */}
                  <div className="space-y-4 mb-6">
                    {comments.length === 0 ? (
                      <p className="text-sm text-muted-foreground">No comments yet</p>
                    ) : (
                      comments.map((comment) => (
                        <div key={comment.id} className="border-b border-gray-200 pb-3 last:border-b-0">
                          <div className="flex items-center justify-between mb-1">
                            <span className="font-medium text-sm text-foreground">{comment.user_name}</span>
                            <span className="text-xs text-muted-foreground">
                              {new Date(comment.created_at).toLocaleString()}
                            </span>
                          </div>
                          <p className="text-sm text-foreground whitespace-pre-wrap">{comment.content}</p>
                        </div>
                      ))
                    )}
                  </div>

                  {/* Comment Form */}
                  {user && (
                    <div className="border-t pt-4">
                      <textarea
                        placeholder="Write a comment..."
                        value={newComment}
                        onChange={(e) => setNewComment(e.target.value)}
                        className="w-full p-3 border border-input rounded-md bg-background text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-none"
                        rows={3}
                      />
                      {commentError && (
                        <p className="text-sm text-red-500 mt-2">{commentError}</p>
                      )}
                      <div className="flex justify-end mt-2">
                        <Button 
                          onClick={postComment} 
                          disabled={isSubmittingComment || !newComment.trim()}
                          size="sm"
                        >
                          {isSubmittingComment ? 'Posting...' : 'Post Comment'}
                        </Button>
                      </div>
                    </div>
                  )}
                  {!user && (
                    <p className="text-sm text-muted-foreground border-t pt-4">
                      <a href="/login" className="text-primary hover:underline">Log in</a> to leave a comment.
                    </p>
                  )}
                </CardContent>
              </Card>
            </div>
          </div>
        </div>

        {/* Edit Date Dialog */}
        <Dialog open={isEditingDate} onOpenChange={setIsEditingDate}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Edit Photo Date</DialogTitle>
              <DialogDescription>
                Enter the photo date in one of these formats: YYYY, YYYY-MM, or YYYY-MM-DD. Leave empty if unknown.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <Input
                placeholder="e.g., 1989, 1989-06, or 1989-06-15"
                value={editDate}
                onChange={(e) => {
                  setEditDate(e.target.value);
                  setDateError('');
                }}
                autoFocus
              />
              {dateError && (
                <p className="text-sm text-red-500">{dateError}</p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsEditingDate(false)}>
                Cancel
              </Button>
              <Button onClick={updatePhotoDate}>
                Save
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Edit Description Dialog */}
        <Dialog open={isEditingDescription} onOpenChange={setIsEditingDescription}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Edit Photo Description</DialogTitle>
              <DialogDescription>
                Enter a description for this photo.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <textarea
                placeholder="Enter description..."
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
                className="w-full h-24 p-3 border border-zinc-700 rounded-lg bg-zinc-800 text-white focus:outline-none focus:border-violet-500 resize-none"
                autoFocus
              />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsEditingDescription(false)}>
                Cancel
              </Button>
              <Button onClick={updatePhotoDescription}>
                Save
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Add Tag Dialog */}
        <Dialog open={isTagDialogOpen} onOpenChange={setIsTagDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Tag to Photo</DialogTitle>
              <DialogDescription>
                Click on the thumbnail to set tag position. Start typing to find existing tags or create a new one.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              {/* Thumbnail preview with position marker */}
              <div 
                className="relative w-full bg-black rounded-lg overflow-hidden cursor-crosshair"
                onClick={(e) => {
                  const rect = e.currentTarget.getBoundingClientRect();
                  const x = ((e.clientX - rect.left) / rect.width) * 100;
                  const y = ((e.clientY - rect.top) / rect.height) * 100;
                  setTagPosition({ x, y });
                  // Focus the tag name input after setting the position
                  setTimeout(() => {
                    tagNameInputRef.current?.focus();
                  }, 100);
                }}
              >
                <img 
                  src={`/thumbnails/${id}?v=${photo?.updated_at || Date.now()}`} 
                  alt="Thumbnail preview" 
                  className="w-full h-auto object-contain"
                />
                {/* Pulsing marker with alternating black/white for visibility */}
                <div 
                  className="absolute w-5 h-5 transform -translate-x-1/2 -translate-y-1/2"
                  style={{ left: `${tagPosition.x}%`, top: `${tagPosition.y}%` }}
                >
                  {/* Outer ring that pulses */}
                  <div className="absolute inset-0 w-full h-full rounded-full border-2 border-white animate-pulse" />
                  {/* Inner circle that alternates */}
                  <div className="absolute inset-1 w-3 h-3 rounded-full bg-white animate-pulse" />
                  {/* Center dot */}
                  <div className="absolute inset-2 w-1.5 h-1.5 rounded-full bg-black animate-pulse" />
                </div>
              </div>
              
              {/* Tag name input with autocomplete */}
              <div className="relative">
                <Input
                  ref={tagNameInputRef}
                  placeholder="Type to search or create tag..."
                  value={newTagName}
                  onChange={(e) => {
                    setNewTagName(e.target.value);
                    setTagError('');
                  }}
                  onFocus={() => setShowSuggestions(newTagName.trim().length > 0)}
                  onBlur={() => setTimeout(() => setShowSuggestions(false), 200)}
                />
                {tagError && (
                  <p className="text-sm text-red-500 mt-1">{tagError}</p>
                )}
                
                {/* Autocomplete dropdown */}
                {showSuggestions && tagSuggestions.length > 0 && (
                  <div className="absolute z-50 w-full mt-1 bg-background border border-input rounded-md shadow-lg max-h-48 overflow-y-auto">
                    {tagSuggestions.map((tag) => (
                      <div
                        key={tag.id}
                        className="px-3 py-2 hover:bg-accent cursor-pointer text-sm"
                        onClick={() => {
                          setNewTagName(tag.name);
                          setShowSuggestions(false);
                        }}
                      >
                        {tag.name}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setIsTagDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={handleAddTag} disabled={isSubmittingTag || !newTagName.trim()}>
                {isSubmittingTag ? 'Adding...' : 'Add Tag'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
