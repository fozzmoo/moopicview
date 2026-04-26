import React, { useState, useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import axios from 'axios';
import { Folder, Search, ArrowLeft } from 'lucide-react';
import { usePath } from '../context/PathContext';
import { Navbar } from '../components/navbar';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';

export default function Browse() {
  const location = useLocation();
  const [view, setView] = useState<'collections' | 'browse'>('collections');
  const [collections, setCollections] = useState<any[]>([]);
  const [directories, setDirectories] = useState<any[]>([]);
  const [photos, setPhotos] = useState<any[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);
  
  const { pathStack, currentPath, setPathStack, setCurrentPath, setCurrentPhotos, addToPathStack, goBackInPath, resetPath } = usePath();

  useEffect(() => {
    fetchCollections();
  }, []);

  // Handle URL parameters for browsing specific paths
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const pathParam = params.get('path');
    
    if (pathParam) {
      browsePath(pathParam);
    } else {
      fetchCollections();
    }
  }, [location.search]);

  const fetchCollections = async () => {
    setLoading(true);
    try {
      const res = await axios.get('/api/collections');
      setCollections(res.data);
      setView('collections');
    } catch (err) {
      console.error('Failed to fetch collections:', err);
    }
    setLoading(false);
  };

  const browsePath = async (path: string) => {
    setLoading(true);
    try {
      const res = await axios.get(`/api/browse?path=${encodeURIComponent(path)}`);
      setDirectories(res.data.directories);
      setPhotos(res.data.photos);
      setCurrentPhotos(res.data.photos);
      setCurrentPath(path);
      setView('browse');
    } catch (err) {
      console.error(err);
    }
    setLoading(false);
  };

  const handleDirectoryClick = (dirPath: string) => {
    addToPathStack({ path: currentPath, name: getBreadcrumbs(currentPath) });
    browsePath(dirPath);
  };

  const handleBreadcrumbClick = (index: number) => {
    const newPath = pathStack[index].path;
    setPathStack(pathStack.slice(0, index));
    if (newPath) {
      browsePath(newPath);
    } else {
      setView('collections');
      fetchCollections();
    }
  };

  const handleBack = () => {
    if (pathStack.length > 0) {
      const prev = pathStack[pathStack.length - 1];
      goBackInPath();
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

  const filteredPhotos = photos.filter(photo =>
    photo.filename.toLowerCase().includes(search.toLowerCase()) ||
    (photo.photo_date && photo.photo_date.includes(search))
  );

  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <div className="container mx-auto px-4 py-8">
        {view === 'collections' && (
          <>
            <div className="mb-8">
              <h1 className="text-4xl font-bold tracking-tight text-foreground mb-2">Photo Collections</h1>
              <p className="text-muted-foreground">Explore your photo library</p>
            </div>
            {loading && <p className="text-muted-foreground">Loading...</p>}
            {!loading && collections.length === 0 && (
              <Card>
                <CardContent className="p-6">
                  <p className="text-muted-foreground">No collections configured</p>
                </CardContent>
              </Card>
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {collections.map((col, i) => (
                <Card key={i} className="group hover:shadow-lg transition-all cursor-pointer" onClick={() => browsePath(col.path)}>
                  <CardHeader>
                    <CardTitle className="capitalize text-2xl">{col.type}</CardTitle>
                    <CardDescription>{col.count} photos</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Folder className="h-4 w-4" />
                      <span>Browse collection</span>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </>
        )}

        {view === 'browse' && (
          <>
            <div className="mb-8">
              <div className="flex items-center gap-4 mb-6">
                {pathStack.length > 0 && (
                  <Button variant="ghost" onClick={handleBack}>
                    <ArrowLeft className="mr-2 h-4 w-4" />
                    Back
                  </Button>
                )}
                <nav className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Link to="/collections" className="hover:text-foreground" onClick={(e) => { e.preventDefault(); resetPath(); fetchCollections(); }}>
                    Collections
                  </Link>
                  {pathStack.map((item, i) => (
                    <React.Fragment key={i}>
                      <span className="text-muted-foreground/50">/</span>
                      <button onClick={() => handleBreadcrumbClick(i)} className="hover:text-foreground">
                        {item.name}
                      </button>
                    </React.Fragment>
                  ))}
                  {currentPath && (
                    <>
                      <span className="text-muted-foreground/50">/</span>
                      <span className="text-foreground font-medium">{getBreadcrumbs(currentPath)}</span>
                    </>
                  )}
                </nav>
              </div>
              <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-8">
                <h1 className="text-3xl font-bold text-foreground">{getBreadcrumbs(currentPath)}</h1>
                <div className="relative w-full sm:w-64">
                  <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                  <input
                    type="text"
                    placeholder="Search photos..."
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="w-full pl-10 pr-4 py-2 border border-input rounded-md bg-background text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                  />
                </div>
              </div>
            </div>

            {loading && <p className="text-muted-foreground">Loading...</p>}
            {!loading && directories.length === 0 && photos.length === 0 && (
              <Card>
                <CardContent className="p-6">
                  <p className="text-muted-foreground">This folder is empty</p>
                </CardContent>
              </Card>
            )}

            {directories.length > 0 && (
              <div className="mb-8">
                <h2 className="text-xl font-semibold text-foreground mb-4">Folders</h2>
                <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
                  {directories.map((dir) => (
                    <Card key={dir.path} className="group hover:shadow-lg transition-all cursor-pointer" onClick={() => handleDirectoryClick(dir.path)}>
                      <CardContent className="p-4">
                        <div className="flex flex-col items-center gap-2 text-center">
                          <Folder className="h-12 w-12 text-muted-foreground group-hover:text-primary transition-colors" />
                          <span className="font-medium text-sm text-foreground">{dir.name}</span>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            )}

            {filteredPhotos.length > 0 && (
              <div>
                <div className="flex items-center gap-2 mb-4">
                  <h2 className="text-xl font-semibold text-foreground">Photos</h2>
                  <Badge variant="secondary">{filteredPhotos.length}</Badge>
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                  {filteredPhotos.map((photo) => (
                    <Link key={photo.id} to={`/photo/${photo.id}`}>
                      <Card className="group hover:shadow-xl transition-all overflow-hidden">
                        <div className="relative aspect-square overflow-hidden bg-muted">
                          <img
                            src={photo.url}
                            alt={photo.filename}
                            className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                          />
                        </div>
                        <CardContent className="p-3">
                          <p className="text-sm font-medium truncate">{photo.filename}</p>
                          <p className="text-xs text-muted-foreground mt-1">
                            {photo.photo_date || 'Unknown date'}
                          </p>
                        </CardContent>
                      </Card>
                    </Link>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
