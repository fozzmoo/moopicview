import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import axios from 'axios';

export default function Browse() {
  const [photos, setPhotos] = useState([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);

  const fetchPhotos = () => {
    setLoading(true);
    axios.get('/api/photos')
      .then(res => setPhotos(res.data))
      .catch(err => console.error(err))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchPhotos();
  }, []);

  return (
    <div className="min-h-screen bg-zinc-950 text-white p-8">
      <nav className="flex gap-6 mb-8 border-b border-zinc-800 pb-4">
        <Link to="/browse" className="text-violet-400 hover:text-violet-300">Browse</Link>
        <Link to="/account" className="hover:text-zinc-300">Account</Link>
        <Link to="/admin" className="hover:text-zinc-300">Admin</Link>
      </nav>
      <div className="flex justify-between items-center mb-8">
        <h1 className="text-3xl font-bold">Photo Collections</h1>
        <input
          type="text"
          placeholder="Search..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-2 text-white w-80"
        />
      </div>
      {loading && <p>Loading photos...</p>}
      {!loading && photos.length === 0 && <p className="text-zinc-400">No photos indexed yet. Background scan is running.</p>}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {photos.map((photo: any) => (
          <Link key={photo.id} to={`/photo/${photo.id}`} className="bg-zinc-900 rounded-xl overflow-hidden cursor-pointer hover:ring-2 hover:ring-violet-500 transition">
            <img src={photo.url} alt={photo.filename} className="w-full h-48 object-cover" />
            <div className="p-3">
              <p className="text-sm font-medium truncate">{photo.filename}</p>
              <p className="text-xs text-zinc-500">{photo.description || photo.collection}</p>
            </div>
          </Link>
        ))}
      </div>
    </div>
  );
}
