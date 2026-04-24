import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import axios from 'axios';

export default function PhotoView() {
  const { id } = useParams();
  const [photo, setPhoto] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    axios.get(`/api/photos/${id}`)
      .then(res => {
        setPhoto(res.data);
        setLoading(false);
      })
      .catch(err => {
        console.error(err);
        setLoading(false);
      });
  }, [id]);

  if (loading) return <div className="min-h-screen bg-black text-white p-8">Loading...</div>;
  if (!photo) return <div className="min-h-screen bg-black text-white p-8">Photo not found</div>;

  return (
    <div className="min-h-screen bg-black text-white p-8">
      <div className="max-w-6xl mx-auto">
        <Link to="/browse" className="inline-flex items-center gap-2 text-zinc-400 hover:text-white mb-8">
          ← Back to Browse
        </Link>
        <div className="flex flex-col lg:flex-row gap-8">
          <div className="flex-1">
            <img src={photo.content_url} alt={photo.filename} className="max-h-[70vh] mx-auto rounded-2xl shadow-2xl" />
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
            </div>

            <div className="mt-10 pt-6 border-t border-zinc-700">
              <div className="text-xs uppercase tracking-widest text-zinc-500 mb-4">Tags</div>
              <div className="flex flex-wrap gap-2">
                {/* TODO: dynamic tags */}
                <span className="bg-zinc-800 text-xs px-3 py-1 rounded-full">vacation</span>
                <span className="bg-zinc-800 text-xs px-3 py-1 rounded-full">family</span>
              </div>
            </div>

            <div className="mt-8">
              <div className="text-xs uppercase tracking-widest text-zinc-500 mb-4">Comments</div>
              <div className="text-sm text-zinc-400">No comments yet.</div>
              {/* TODO: comment list and form */}
            </div>

            <button className="mt-8 w-full bg-amber-600 hover:bg-amber-700 py-3 rounded-lg text-sm">
              Propose Edit
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
