import { useState, useEffect } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import api from '@/lib/api';
import { Folder, Search, ArrowLeft } from 'lucide-react';
import { Navbar } from '../components/navbar';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { formatDate } from '@/lib/dateUtils';
import ProgressiveImage from '@/components/ProgressiveImage';

export default function Collections() {
  const navigate = useNavigate();
  const params = useParams();
  const [view, setView] = useState<'collections' | 'folders'>('collections');
  const [collections, setCollections] = useState<any[]>([]);
  const [currentFolder, setCurrentFolder] = useState<any>(null);
  const [directories, setDirectories] = useState<any[]>([]);
  const [photos, setPhotos] = useState<any[]>([]);
  const [breadcrumbs, setBreadcrumbs] = useState<any[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const folderId = params.id ? parseInt(params.id) : null;
    
    if (folderId) {
      loadFolder(folderId);
    } else {
      loadCollections();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.id]);

  const loadCollections = async () => {
    setLoading(true);
    try {
      const res = await api.get('/api/collections');
      setCollections(res.data);
      setCurrentFolder(null);
      setDirectories([]);
      setPhotos([]);
      setBreadcrumbs([]);
      setView('collections');
    } catch (err) {
      console.error('Failed to fetch collections:', err);
    }
    setLoading(false);
  };

  const loadFolder = async (folderId: number) => {
    setLoading(true);
    try {
      const res = await api.get(`/api/collections/${folderId}`);
      setCurrentFolder(res.data.folder);
      setDirectories(res.data.directories || []);
      setPhotos(res.data.photos || []);
      setBreadcrumbs(res.data.breadcrumbs || []);
      setView('folders');
    } catch (err) {
      console.error('Failed to load folder:', err);
    }
    setLoading(false);
  };

  const handleFolderClick = (folderId: number) => {
    navigate(`/collections/${folderId}`);
  };

  const handleBack = () => {
    navigate('/collections');
  };

  const filteredPhotos = (photos || []).filter(photo =>
    photo.filename.toLowerCase().includes(search.toLowerCase()) ||
    (photo.photo_date && photo.photo_date.includes(search))
  );

  const getCollectionTypeLabel = (type: string) => {
    return type === 'digital' ? 'Digital' : 'Scanned';
  };

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
              {collections.map((col) => (
                <Card key={col.id} className="group hover:shadow-lg transition-all cursor-pointer" onClick={() => handleFolderClick(col.id)}>
                  <CardHeader>
                    <CardTitle className="capitalize text-2xl">{col.name}</CardTitle>
                    <CardDescription>{col.count} photos ({getCollectionTypeLabel(col.type)})</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Folder className="h-4 w-4" />
                      <span>View collection</span>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          </>
        )}

        {view === 'folders' && currentFolder && (
          <>
            <div className="mb-8">
              <div className="flex items-center gap-4 mb-6">
                <Button variant="ghost" onClick={handleBack}>
                  <ArrowLeft className="mr-2 h-4 w-4" />
                  Back to Collections
                </Button>
                <nav className="flex items-center gap-2 text-sm text-muted-foreground">
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
                  <span className="text-muted-foreground ml-1">
                    ({getCollectionTypeLabel(currentFolder.collection_type)})
                  </span>
                </nav>
              </div>
              <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-8">
                <h1 className="text-3xl font-bold text-foreground">{currentFolder.name}</h1>
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
                    <Card key={dir.id} className="group hover:shadow-lg transition-all cursor-pointer" onClick={() => handleFolderClick(dir.id)}>
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
                            <ProgressiveImage
                              src={photo.url}
                              thumbnail={`/thumbnails/${photo.id}`}
                              alt={photo.filename}
                              className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                            />
                          </div>
                          <CardContent className="p-3">
                            <p className="text-sm font-medium truncate">{photo.filename}</p>
                            <p className="text-xs text-muted-foreground mt-1">
                              {formatDate(photo.photo_date, photo.date_precision)}
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
