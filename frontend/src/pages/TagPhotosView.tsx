import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import api from '@/lib/api';
import { Navbar } from '../components/navbar';
import { Card, CardContent } from '../components/ui/card';
import { Calendar, Tag, ArrowLeft } from 'lucide-react';
import { formatDate } from '@/lib/dateUtils';

export default function TagPhotosView() {
  const { tagId } = useParams();
  const [photos, setPhotos] = useState<any[]>([]);
  const [tagName, setTagName] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchTagPhotos = async () => {
      if (!tagId) return;
      
      try {
        setLoading(true);
        const response = await api.get(`/api/tags/${tagId}/photos`);
        setPhotos(response.data.photos || []);
        setTagName(response.data.tag_name || '');
      } catch (err) {
        console.error('Failed to fetch tag photos:', err);
        setError('Failed to load photos for this tag');
      } finally {
        setLoading(false);
      }
    };

    fetchTagPhotos();
  }, [tagId]);

  if (loading) {
    return (
      <div className="min-h-screen bg-background">
        <Navbar />
        <div className="container mx-auto px-4 py-6">
          <div className="flex items-center justify-center h-64">
            <p>Loading photos...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-background">
        <Navbar />
        <div className="container mx-auto px-4 py-6">
          <div className="flex items-center justify-center h-64">
            <p className="text-red-500">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <div className="container mx-auto px-4 py-6">
        {/* Header */}
        <div className="flex items-center gap-4 mb-6">
          <Link 
            to="/tags" 
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
          >
            <ArrowLeft className="h-4 w-4" />
            <span>Back to Tags</span>
          </Link>
        </div>

        {/* Tag Info */}
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-foreground flex items-center gap-2">
            <Tag className="h-6 w-6" />
            {tagName}
          </h1>
          <p className="text-muted-foreground mt-1">
            {photos.length} photo{photos.length !== 1 ? 's' : ''}
          </p>
        </div>

        {/* Photo Grid */}
        {photos.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-muted-foreground">No photos found with this tag.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            {photos.map((photo) => (
              <Card 
                key={photo.id} 
                className="overflow-hidden cursor-pointer hover:shadow-lg transition-shadow"
              >
                <Link to={`/photo/${photo.id}`}>
                  <div className="aspect-square bg-gray-100">
                    <img
                      src={photo.content_url}
                      alt={photo.filename}
                      className="w-full h-full object-cover"
                    />
                  </div>
                  <CardContent className="p-3">
                    <p className="text-sm font-medium truncate">{photo.filename}</p>
                    <div className="flex items-center gap-2 mt-1 text-xs text-muted-foreground">
                      <Calendar className="h-3 w-3" />
                      <span>{formatDate(photo.photo_date, photo.date_precision)}</span>
                    </div>
                  </CardContent>
                </Link>
              </Card>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
