import { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import api from '@/lib/api';
import { Calendar, Folder as FolderIcon, MapPin, Download, ChevronLeft, ChevronRight, Edit2 } from 'lucide-react';
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
  const [loading, setLoading] = useState(true);
  const [isEditingDate, setIsEditingDate] = useState(false);
  const [editDate, setEditDate] = useState('');
  const [dateError, setDateError] = useState('');
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const [editDescription, setEditDescription] = useState('');
  const [newComment, setNewComment] = useState('');
  const [commentError, setCommentError] = useState('');
  const [isSubmittingComment, setIsSubmittingComment] = useState(false);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const photoRes = await api.get(`/api/photos/${id}`);
        setPhoto(photoRes.data.photo);
        setBreadcrumbs(photoRes.data.breadcrumbs || []);
        setComments(photoRes.data.comments || []);
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
              <Card className="overflow-hidden">
                <div className="relative bg-black/5 flex items-center justify-center min-h-[400px]">
                  {/* Previous/Next Navigation */}
                  {photo.prev_photo_id && (
                    <Link
                      to={`/photo/${photo.prev_photo_id}`}
                      className="absolute left-2 top-1/2 -translate-y-1/2 z-10 p-2 bg-black/50 hover:bg-black/70 rounded-full transition-colors"
                    >
                      <ChevronLeft className="h-6 w-6 text-white" />
                    </Link>
                  )}
                  {photo.next_photo_id && (
                    <Link
                      to={`/photo/${photo.next_photo_id}`}
                      className="absolute right-2 top-1/2 -translate-y-1/2 z-10 p-2 bg-black/50 hover:bg-black/70 rounded-full transition-colors"
                    >
                      <ChevronRight className="h-6 w-6 text-white" />
                    </Link>
                  )}
                  
                  <ProgressiveImage
                    src={photo.content_url}
                    thumbnail={`/thumbnails/${id}`}
                    alt={photo.filename}
                    className="max-h-[70vh] w-auto rounded-lg"
                  />
                </div>
              </Card>
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
      </div>
    </div>
  );
}
