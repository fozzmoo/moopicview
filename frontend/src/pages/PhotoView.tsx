import React, { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import axios from 'axios';
import { ChevronLeft, ChevronRight, Calendar, Folder as FolderIcon, MapPin, Download } from 'lucide-react';
import { usePath } from '../context/PathContext';
import { Navbar } from '../components/navbar';
import { Card, CardContent } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';

export default function PhotoView() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [photo, setPhoto] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  
  const { pathStack, currentPath, currentPhotos } = usePath();
  const photos = currentPhotos;

  useEffect(() => {
    const fetchData = async () => {
      try {
        const photoRes = await axios.get(`/api/photos/${id}`);
        setPhoto(photoRes.data);
      } catch (err) {
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [id]);

  const currentIndex = photos.findIndex(p => p.id === parseInt(id || '0'));
  const prevPhoto = currentIndex > 0 ? photos[currentIndex - 1] : null;
  const nextPhoto = currentIndex < photos.length - 1 ? photos[currentIndex + 1] : null;

  const getBreadcrumbs = (path: string): string => {
    const parts = path.split('/').filter(Boolean);
    if (parts.length === 0) return 'Root';
    return parts[parts.length - 1];
  };

  const navigateToPath = (path: string) => {
    navigate(`/collections?path=${encodeURIComponent(path)}`);
  };

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

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft' && prevPhoto) {
        navigate(`/photo/${prevPhoto.id}`);
      } else if (e.key === 'ArrowRight' && nextPhoto) {
        navigate(`/photo/${nextPhoto.id}`);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [prevPhoto, nextPhoto, navigate]);

  if (loading) return <div className="min-h-screen bg-background flex items-center justify-center">Loading...</div>;
  if (!photo) return <div className="min-h-screen bg-background flex items-center justify-center">Photo not found</div>;

  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <div className="container mx-auto px-4 py-6">
        <nav className="flex items-center gap-2 text-sm text-muted-foreground mb-6">
          <Link to="/collections" className="hover:text-foreground">Collections</Link>
          {pathStack.map((item, i) => (
            <React.Fragment key={i}>
              <span className="text-muted-foreground/50">/</span>
              <Link to="/collections" className="hover:text-foreground" onClick={(e) => { e.preventDefault(); navigateToPath(item.path); }}>
                {item.name}
              </Link>
            </React.Fragment>
          ))}
          {currentPath && pathStack.length > 0 && (
            <>
              <span className="text-muted-foreground/50">/</span>
              <span className="text-foreground font-medium">{getBreadcrumbs(currentPath)}</span>
            </>
          )}
        </nav>

        <div className="flex items-start gap-4 lg:gap-8">
          {prevPhoto && (
            <Button
              variant="outline"
              size="icon"
              onClick={() => navigate(`/photo/${prevPhoto.id}`)}
              className="hidden lg:flex flex-shrink-0 mt-24"
              title="Previous photo (←)"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
          )}
          
          <div className="flex-1 flex flex-col gap-6">
            <div className="flex-1">
              <Card className="overflow-hidden">
                <div className="bg-black/5 flex items-center justify-center min-h-[400px]">
                  <img
                    src={photo.content_url}
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
                    <p className="text-sm text-muted-foreground">{photo.description || 'No description'}</p>
                  </div>

                  <div className="space-y-3 pt-4 border-t">
                    <div className="flex items-center gap-3 text-sm">
                      <FolderIcon className="h-4 w-4 text-muted-foreground" />
                      <span className="font-medium capitalize text-foreground">{photo.collection}</span>
                    </div>
                    <div className="flex items-center gap-3 text-sm">
                      <Calendar className="h-4 w-4 text-muted-foreground" />
                      <span className="text-foreground">{photo.photo_date || 'Unknown date'}</span>
                    </div>
                    <div className="flex items-center gap-3 text-sm">
                      <MapPin className="h-4 w-4 text-muted-foreground" />
                      <span className="text-muted-foreground">Location not set</span>
                    </div>
                  </div>

                  <div className="flex items-center justify-between pt-4 border-t">
                    <span className="text-sm text-muted-foreground">Photo</span>
                    <Badge variant="secondary">
                      {currentIndex + 1} / {photos.length}
                    </Badge>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardContent className="p-6">
                  <h3 className="font-semibold mb-3 text-sm text-foreground">Tags</h3>
                  <div className="flex flex-wrap gap-2">
                    <Badge variant="outline">vacation</Badge>
                    <Badge variant="outline">family</Badge>
                    <Badge variant="outline">memories</Badge>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardContent className="p-6">
                  <h3 className="font-semibold mb-3 text-sm text-foreground">Actions</h3>
                  <div className="space-y-2">
                    <Button variant="outline" className="w-full justify-start">
                      Add to favorites
                    </Button>
                    <Button variant="outline" className="w-full justify-start" onClick={handleDownload}>
                      <Download className="mr-2 h-4 w-4" />
                      Download
                    </Button>
                    <Button variant="outline" className="w-full justify-start">
                      Share
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
          
          {nextPhoto && (
            <Button
              variant="outline"
              size="icon"
              onClick={() => navigate(`/photo/${nextPhoto.id}`)}
              className="hidden lg:flex flex-shrink-0 mt-24"
              title="Next photo (→)"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          )}
        </div>

        <div className="flex justify-center gap-4 mt-6 lg:hidden">
          {prevPhoto && (
            <Button
              variant="outline"
              onClick={() => navigate(`/photo/${prevPhoto.id}`)}
            >
              <ChevronLeft className="mr-2 h-4 w-4" />
              Previous
            </Button>
          )}
          {nextPhoto && (
            <Button
              variant="outline"
              onClick={() => navigate(`/photo/${nextPhoto.id}`)}
            >
              Next
              <ChevronRight className="ml-2 h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
