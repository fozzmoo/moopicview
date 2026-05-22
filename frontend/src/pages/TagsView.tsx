import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import api from '@/lib/api';
import { Navbar } from '../components/navbar';
import { Card, CardContent } from '../components/ui/card';
import { Tag, Search } from 'lucide-react';

export default function TagsView() {
  const [tags, setTags] = useState<any[]>([]);
  const [filteredTags, setFilteredTags] = useState<any[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<'name' | 'count'>('name');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchTags = async () => {
      try {
        setLoading(true);
        const response = await api.get('/api/tags');
        setTags(response.data || []);
        setFilteredTags(response.data || []);
      } catch (err) {
        console.error('Failed to fetch tags:', err);
        setError('Failed to load tags');
      } finally {
        setLoading(false);
      }
    };

    fetchTags();
  }, []);

  useEffect(() => {
    let result = tags;
    if (searchQuery.trim() !== '') {
      const query = searchQuery.toLowerCase();
      result = tags.filter(tag => tag.name.toLowerCase().includes(query));
    }
    result = [...result].sort((a, b) => {
      if (sortBy === 'name') {
        return a.name.localeCompare(b.name);
      }
      return b.photo_count - a.photo_count;
    });
    setFilteredTags(result);
  }, [searchQuery, tags, sortBy]);

  if (loading) {
    return (
      <div className="min-h-screen bg-background">
        <Navbar />
        <div className="container mx-auto px-4 py-6">
          <div className="flex items-center justify-center h-64">
            <p>Loading tags...</p>
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
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-foreground flex items-center gap-2">
            <Tag className="h-6 w-6" />
            Tags
          </h1>
          <p className="text-muted-foreground mt-1">
            Browse photos by tag
          </p>
        </div>

        {/* Search & Sort */}
        <div className="mb-6 flex flex-col sm:flex-row gap-4">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              placeholder="Search tags..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-10 pr-4 py-2 border border-input rounded-md bg-background focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Sort by:</span>
            <button
              onClick={() => setSortBy('name')}
              className={`px-3 py-1 rounded-md transition-colors ${
                sortBy === 'name'
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
              }`}
            >
              Tag name
            </button>
            <button
              onClick={() => setSortBy('count')}
              className={`px-3 py-1 rounded-md transition-colors ${
                sortBy === 'count'
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
              }`}
            >
              Image count
            </button>
          </div>
        </div>

        {/* Tags Grid */}
        {filteredTags.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-muted-foreground">
              {searchQuery ? 'No tags match your search.' : 'No tags found.'}
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
            {filteredTags.map((tag) => (
              <Link key={tag.id} to={`/tags/${tag.id}`}>
                <Card className="cursor-pointer hover:shadow-lg transition-shadow h-full">
                  <CardContent className="p-4 flex items-center gap-3">
                    <Tag className="h-8 w-8 text-primary flex-shrink-0" />
                    <div className="min-w-0">
                      <p className="font-medium truncate">{tag.name}</p>
                      <p className="text-sm text-muted-foreground">
                        ({tag.photo_count} {tag.photo_count === 1 ? 'image' : 'images'})
                      </p>
                    </div>
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
