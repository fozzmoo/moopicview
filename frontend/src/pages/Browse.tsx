import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import axios from 'axios';
import { Folder } from 'lucide-react';

type PathItem = { path: string; name: string };

export default function Browse() {
  const [view, setView] = useState<'collections' | 'browse'>('collections');
  const [collections, setCollections] = useState<any[]>([]);
  const [currentPath, setCurrentPath] = useState('');
  const [directories, setDirectories] = useState<any[]>([]);
  const [photos, setPhotos] = useState<any[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  const [pathStack, setPathStack] = useState<PathItem[]>([]);

  useEffect(() => {
    fetchCollections();
  }, []);

  const fetchCollections = async () => {
    setLoading(true);
    try {
      const res = await axios.get('/api/collections');
      setCollections(res.data);
      setView('collections');
    } catch (err) {
      console.error(err);
    }
    setLoading(false);
  };

  const browsePath = async (path: string) => {
    setLoading(true);
    try {
      const res = await axios.get(`/api/browse?path=${encodeURIComponent(path)}`);
      setDirectories(res.data.directories);
      setPhotos(res.data.photos);
      setCurrentPath(path);
      setView('browse');
    } catch (err) {
      console.error(err);
    }
    setLoading(false);
  };

  const handleDirectoryClick = (dirPath: string) => {
    setPathStack([...pathStack, { path: currentPath, name: getBreadcrumbs(currentPath) }]);
    browsePath(dirPath);
  };

  const handleBreadcrumbClick = (index: number) => {
    const newPath = pathStack[index].path;
    setPathStack(pathStack.slice(0, index));
    if (newPath) {
      browsePath(newPath);
    } else {
      fetchCollections();
    }
  };

  const handleBack = () => {
    if (pathStack.length > 0) {
      const prev = pathStack[pathStack.length - 1];
      setPathStack(pathStack.slice(0, -1));
      if (prev.path) {
        browsePath(prev.path);
      } else {
        fetchCollections();
      }
    }
  };

  const getBreadcrumbs = (path: string): string => {
    const parts = path.split('/').filter(Boolean);
    if (parts.length === 0) return 'Root';
    return parts[parts.length - 1];
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-white p-8">
      <nav className="flex gap-6 mb-8 border-b border-zinc-800 pb-4">
        <Link to="/browse" className="text-violet-400 hover:text-violet-300" onClick={() => setView('collections')}>
          Browse
        </Link>
        <Link to="/account" className="hover:text-zinc-300">Account</Link>
        <Link to="/admin" className="hover:text-zinc-300">Admin</Link>
      </nav>

      {view === 'collections' && (
        <>
          <h1 className="text-3xl font-bold mb-8">Photo Collections</h1>
          {loading && <p>Loading...</p>}
          {!loading && collections.length === 0 && <p className="text-zinc-400">No collections configured</p>}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {collections.map((col, i) => (
              <button
                key={i}
                onClick={() => browsePath(col.path)}
                className="bg-zinc-900 hover:bg-zinc-800 p-6 rounded-xl text-left transition-colors"
              >
                <h2 className="text-2xl font-bold capitalize">{col.type}</h2>
                <p className="text-zinc-400 mt-2">{col.count} photos</p>
              </button>
            ))}
          </div>
        </>
      )}

      {view === 'browse' && (
        <>
          <div className="flex items-center gap-4 mb-6">
            {pathStack.length > 0 && (
              <button onClick={handleBack} className="text-zinc-400 hover:text-white">
                ← Back
              </button>
            )}
            <div className="flex items-center gap-2 text-zinc-400">
              <Link to="/browse" className="hover:text-white">Browse</Link>
              {pathStack.map((item, i) => (
                <>
                  <span className="text-zinc-600">/</span>
                  <button onClick={() => handleBreadcrumbClick(i)} className="hover:text-white">
                    {item.name}
                  </button>
                </>
              ))}
            </div>
          </div>
          <div className="flex justify-between items-center mb-8">
            <h1 className="text-3xl font-bold">{getBreadcrumbs(currentPath)}</h1>
            <input
              type="text"
              placeholder="Search..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-2 text-white w-80"
            />
          </div>

          {loading && <p>Loading...</p>}
          {!loading && directories.length === 0 && photos.length === 0 && (
            <p className="text-zinc-400">This folder is empty</p>
          )}

          {directories.length > 0 && (
            <div className="mb-8">
              <h2 className="text-xl font-semibold mb-4 text-zinc-300">Folders</h2>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                {directories.map((dir) => (
                  <button
                    key={dir.path}
                    onClick={() => handleDirectoryClick(dir.path)}
                    className="bg-zinc-900 hover:bg-zinc-800 p-4 rounded-xl text-left flex items-center gap-3 transition-colors"
                  >
                    <Folder className="w-8 h-8 text-zinc-400" />
                    <span className="font-medium">{dir.name}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {photos.length > 0 && (
            <div>
              <h2 className="text-xl font-semibold mb-4 text-zinc-300">Photos ({photos.length})</h2>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                {photos.map((photo) => (
                  <Link
                    key={photo.id}
                    to={`/photo/${photo.id}`}
                    className="bg-zinc-900 rounded-xl overflow-hidden cursor-pointer hover:ring-2 hover:ring-violet-500 transition"
                  >
                    <img src={photo.url} alt={photo.filename} className="w-full h-48 object-cover" />
                    <div className="p-3">
                      <p className="text-sm font-medium truncate">{photo.filename}</p>
                      <p className="text-xs text-zinc-500">
                        {photo.photo_date || 'Unknown date'}
                      </p>
                    </div>
                  </Link>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
