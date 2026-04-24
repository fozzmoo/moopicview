import { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import axios from 'axios';
import { ChevronLeft, ChevronRight, ArrowLeft } from 'lucide-react';

export default function PhotoView() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [photo, setPhoto] = useState<any>(null);
  const [photos, setPhotos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [photoRes, listRes] = await Promise.all([
          axios.get(`/api/photos/${id}`),
          axios.get('/api/photos')
        ]);
        setPhoto(photoRes.data);
        setPhotos(listRes.data);
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

  if (loading) return <div className="min-h-screen bg-black text-white p-8">Loading...</div>;
  if (!photo) return <div className="min-h-screen bg-black text-white p-8">Photo not found</div>;

  return (
    <div className="min-h-screen bg-black text-white p-8">
      <div className="max-w-7xl mx-auto">
        <Link to="/browse" className="inline-flex items-center gap-2 text-zinc-400 hover:text-white mb-8">
          <ArrowLeft className="w-4 h-4" /> Back to Browse
        </Link>
        <div className="flex items-start gap-8">
          {prevPhoto && (
            <Link
              to={`/photo/${prevPhoto.id}`}
              className="flex items-center justify-center w-16 h-16 bg-zinc-900 hover:bg-zinc-800 rounded-full flex-shrink-0 mt-24 transition-colors"
              title="Previous photo (←)"
            >
              <ChevronLeft className="w-8 h-8" />
            </Link>
          )}
          <div className="flex-1 flex flex-col lg:flex-row gap-8">
            <div className="flex-1">
              <img
                src={photo.content_url}
                alt={photo.filename}
                className="max-h-[70vh] mx-auto rounded-2xl shadow-2xl"
              />
            </div>
            <div className="w-full lg:w-96 bg-zinc-900 rounded-2xl p-6">
              <h1 className="text-2xl font-bold mb-4">{photo.filename}</h1>
              <p className="text-zinc-400 mb-6">{photo.description}</p>

              <div className="space-y-6">
                <div>
                  <div className="text-xs uppercase tracking-widest text-zinc-500 mb-2">Collection</div>
                  <div className="font-medium">{photo.collection}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-widest text-zinc-500 mb-2">Date</div>
                  <div className="font-medium">{photo.scan_date || 'Unknown'}</div>
                </div>
                <div>
                  <div className="text-xs uppercase tracking-widest text-zinc-500 mb-2">Position</div>
                  <div className="font-medium text-zinc-400">
                    {currentIndex + 1} / {photos.length}
                  </div>
                </div>
              </div>

              <div className="mt-10 pt-6 border-t border-zinc-700">
                <div className="text-xs uppercase tracking-widest text-zinc-500 mb-4">Tags</div>
                <div className="flex flex-wrap gap-2">
                  <span className="bg-zinc-800 text-xs px-3 py-1 rounded-full">vacation</span>
                  <span className="bg-zinc-800 text-xs px-3 py-1 rounded-full">family</span>
                </div>
              </div>

              <div className="mt-8">
                <div className="text-xs uppercase tracking-widest text-zinc-500 mb-4">Comments</div>
                <div className="text-sm text-zinc-400">No comments yet.</div>
              </div>

              <button className="mt-8 w-full bg-amber-600 hover:bg-amber-700 py-3 rounded-lg text-sm transition-colors">
                Propose Edit
              </button>
            </div>
          </div>
          {nextPhoto && (
            <Link
              to={`/photo/${nextPhoto.id}`}
              className="flex items-center justify-center w-16 h-16 bg-zinc-900 hover:bg-zinc-800 rounded-full flex-shrink-0 mt-24 transition-colors"
              title="Next photo (→)"
            >
              <ChevronRight className="w-8 h-8" />
            </Link>
          )}
        </div>
      </div>
    </div>
  );
}
