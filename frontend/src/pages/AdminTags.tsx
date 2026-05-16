import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import api from '@/lib/api';
import { Navbar } from '../components/navbar';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from '@/components/ui/input';
import { ArrowLeft, Plus, Edit2, Trash2, Tag } from 'lucide-react';

interface TagData {
  id: number;
  name: string;
  photo_count: number;
}

export default function AdminTags() {
  const [tags, setTags] = useState<TagData[]>([]);
  const [loading, setLoading] = useState(true);

  // Create dialog state
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [newTagName, setNewTagName] = useState('');
  const [createError, setCreateError] = useState('');

  // Edit dialog state
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingTag, setEditingTag] = useState<TagData | null>(null);
  const [editTagName, setEditTagName] = useState('');
  const [editError, setEditError] = useState('');

  // Delete dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingTagId, setDeletingTagId] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState('');

  useEffect(() => {
    fetchTags();
  }, []);

  const fetchTags = async () => {
    setLoading(true);
    try {
      const res = await api.get('/api/admin/tags');
      setTags(res.data || []);
    } catch (err) {
      console.error('Failed to fetch tags:', err);
    }
    setLoading(false);
  };

  const openCreateDialog = () => {
    setNewTagName('');
    setCreateError('');
    setCreateDialogOpen(true);
  };

  const createTag = async () => {
    if (!newTagName.trim()) {
      setCreateError('Tag name is required');
      return;
    }
    try {
      await api.post('/api/admin/tags', { name: newTagName.trim() });
      setCreateDialogOpen(false);
      setNewTagName('');
      fetchTags();
    } catch (err: any) {
      console.error('Failed to create tag:', err);
      setCreateError(err.response?.data || 'Failed to create tag. Please try again.');
    }
  };

  const openEditDialog = (tag: TagData) => {
    setEditingTag(tag);
    setEditTagName(tag.name);
    setEditError('');
    setEditDialogOpen(true);
  };

  const updateTag = async () => {
    if (!editingTag || !editTagName.trim()) {
      setEditError('Tag name is required');
      return;
    }
    try {
      await api.put(`/api/admin/tags/${editingTag.id}`, { name: editTagName.trim() });
      setEditDialogOpen(false);
      setEditingTag(null);
      fetchTags();
    } catch (err: any) {
      console.error('Failed to update tag:', err);
      setEditError(err.response?.data || 'Failed to update tag. Please try again.');
    }
  };

  const openDeleteDialog = (tagId: number) => {
    setDeletingTagId(tagId);
    setDeleteError('');
    setDeleteDialogOpen(true);
  };

  const deleteTag = async () => {
    if (!deletingTagId) {
      setDeleteError('No tag selected');
      return;
    }
    try {
      await api.delete(`/api/admin/tags/${deletingTagId}`);
      setDeleteDialogOpen(false);
      setDeletingTagId(null);
      fetchTags();
    } catch (err: any) {
      console.error('Failed to delete tag:', err);
      setDeleteError(err.response?.data || 'Failed to delete tag. Please try again.');
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <Navbar />
        <div>Loading...</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-background">
      <Navbar />
      <div className="container mx-auto p-6">
        <div className="flex items-center gap-4 mb-6">
          <Link to="/admin">
            <Button variant="outline" size="sm">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Admin
            </Button>
          </Link>
        </div>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Tag Administration</CardTitle>
              <CardDescription>Manage photo tags - create, edit, or delete tags</CardDescription>
            </div>
            <Button onClick={openCreateDialog}>
              <Plus className="h-4 w-4 mr-2" />
              Create Tag
            </Button>
          </CardHeader>
          <CardContent>
            {tags.length === 0 ? (
              <p className="text-muted-foreground">
                No tags created yet. Click "Create Tag" to add your first tag.
              </p>
            ) : (
              <div className="space-y-2">
                {tags.map(tag => (
                  <div key={tag.id} className="flex items-center justify-between p-4 border rounded-lg">
                    <div className="flex items-center gap-3">
                      <Tag className="h-4 w-4 text-muted-foreground" />
                      <span className="font-medium">{tag.name}</span>
                      <Badge variant="secondary">
                        {tag.photo_count} {tag.photo_count === 1 ? 'image' : 'images'}
                      </Badge>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" size="sm" onClick={() => openEditDialog(tag)}>
                        <Edit2 className="h-4 w-4 mr-1" />
                        Edit
                      </Button>
                      <Button variant="destructive" size="sm" onClick={() => openDeleteDialog(tag.id)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Create Tag Dialog */}
        <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create New Tag</DialogTitle>
              <DialogDescription>
                Enter a name for the new tag.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <Input
                placeholder="Tag name"
                value={newTagName}
                onChange={(e) => setNewTagName(e.target.value)}
                autoFocus
              />
              {createError && (
                <p className="text-sm text-red-500">{createError}</p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={createTag}>
                Create Tag
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Edit Tag Dialog */}
        <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Edit Tag</DialogTitle>
              <DialogDescription>
                Update the tag name.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <Input
                placeholder="Tag name"
                value={editTagName}
                onChange={(e) => setEditTagName(e.target.value)}
                autoFocus
              />
              {editError && (
                <p className="text-sm text-red-500">{editError}</p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={updateTag}>
                Save Changes
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Delete Tag Dialog */}
        <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Delete Tag</DialogTitle>
              <DialogDescription>
                Are you sure you want to delete this tag? It will be removed from all photos. This action cannot be undone.
              </DialogDescription>
            </DialogHeader>
            {deleteError && (
              <p className="text-sm text-red-500">{deleteError}</p>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={deleteTag}>
                Delete Tag
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}
