import { useState, useEffect } from 'react';
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
import { Checkbox } from '@/components/ui/checkbox';
import { Link } from 'react-router-dom';
import { Settings, ExternalLink } from 'lucide-react';

export default function AdminDashboard() {
  const [users, setUsers] = useState<any[]>([]);
  const [proposedEdits, setProposedEdits] = useState<any[]>([]);
  const [accountRequests, setAccountRequests] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null);
  const [newPassword, setNewPassword] = useState('');
  const [confirmNewPassword, setConfirmNewPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [createUserDialogOpen, setCreateUserDialogOpen] = useState(false);
  const [newUser, setNewUser] = useState({ firstName: '', lastName: '', email: '', password: '', confirmPassword: '', isAdmin: false });
  const [createUserError, setCreateUserError] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [userToDelete, setUserToDelete] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [scanning, setScanning] = useState(false);
  const [scanMessage, setScanMessage] = useState('');

  useEffect(() => {
    fetchAdminData();
  }, []);

  const fetchAdminData = async () => {
    setLoading(true);
    try {
      const [usersRes, editsRes, requestsRes] = await Promise.all([
        api.get('/api/admin/users'),
        api.get('/api/admin/proposed-edits'),
        api.get('/api/admin/account-requests')
      ]);
      setUsers(usersRes.data);
      setProposedEdits(editsRes.data);
      setAccountRequests(requestsRes.data);
    } catch (err) {
      console.error('Failed to fetch admin data:', err);
    }
    setLoading(false);
  };

  const approveUser = async (userId: number) => {
    try {
      await api.post(`/api/admin/users/${userId}/approve`);
      fetchAdminData(); // Refresh data
    } catch (err) {
      console.error('Failed to approve user:', err);
    }
  };

  const reviewAccountRequest = async (requestId: number, status: string) => {
    try {
      await api.post(`/api/admin/account-requests/${requestId}/review`, { status });
      fetchAdminData(); // Refresh data
    } catch (err) {
      console.error('Failed to review account request:', err);
    }
  };

  const reviewProposedEdit = async (editId: number, status: string) => {
    try {
      await api.post(`/api/admin/proposed-edits/${editId}/review`, { status });
      fetchAdminData(); // Refresh data
    } catch (err) {
      console.error('Failed to review proposed edit:', err);
    }
  };

  const changePassword = async () => {
    if (!selectedUserId || !newPassword) {
      setPasswordError('Please enter a new password');
      return;
    }

    if (newPassword.length < 6) {
      setPasswordError('Password must be at least 6 characters');
      return;
    }

    if (newPassword !== confirmNewPassword) {
      setPasswordError('Passwords do not match');
      return;
    }

    try {
      await api.post(`/api/admin/users/${selectedUserId}/change-password`, { newPassword });
      setPasswordDialogOpen(false);
      setSelectedUserId(null);
      setNewPassword('');
      setConfirmNewPassword('');
    } catch (err) {
      console.error('Failed to change password:', err);
      setPasswordError('Failed to change password. Please try again.');
    }
  };

  const openPasswordDialog = (userId: number) => {
    setSelectedUserId(userId);
    setNewPassword('');
    setConfirmNewPassword('');
    setPasswordError('');
    setPasswordDialogOpen(true);
  };

  const openCreateUserDialog = () => {
    setNewUser({ firstName: '', lastName: '', email: '', password: '', confirmPassword: '', isAdmin: false });
    setCreateUserError('');
    setCreateUserDialogOpen(true);
  };

  const createUser = async () => {
    if (!newUser.firstName || !newUser.lastName || !newUser.email) {
      setCreateUserError('First name, last name, and email are required');
      return;
    }

    // Password handling: must both be empty OR both be filled and match
    const passwordEmpty = !newUser.password;
    const confirmPasswordEmpty = !newUser.confirmPassword;
    
    if (passwordEmpty !== confirmPasswordEmpty) {
      setCreateUserError('Please fill in both password fields or leave both empty');
      return;
    }
    
    if (!passwordEmpty && !confirmPasswordEmpty) {
      if (newUser.password.length < 6) {
        setCreateUserError('Password must be at least 6 characters');
        return;
      }
      if (newUser.password !== newUser.confirmPassword) {
        setCreateUserError('Passwords do not match');
        return;
      }
    }

    try {
      await api.post('/api/admin/users', {
        first_name: newUser.firstName,
        last_name: newUser.lastName,
        email: newUser.email,
        password: newUser.password || null, // Send null if empty
        is_admin: newUser.isAdmin
      });
      setCreateUserDialogOpen(false);
      fetchAdminData(); // Refresh user list
    } catch (err: any) {
      console.error('Failed to create user:', err);
      setCreateUserError(err.response?.data || 'Failed to create user. Please try again.');
    }
  };

  const toggleAdmin = async (userId: number) => {
    try {
      await api.post(`/api/admin/users/${userId}/toggle-admin`);
      fetchAdminData(); // Refresh user list
    } catch (err) {
      console.error('Failed to toggle admin status:', err);
    }
  };

  const openDeleteDialog = (userId: number) => {
    setUserToDelete(userId);
    setDeleteError('');
    setDeleteDialogOpen(true);
  };

  const deleteUser = async () => {
    if (!userToDelete) {
      setDeleteError('No user selected');
      return;
    }

    try {
      await api.delete(`/api/admin/users/${userToDelete}/delete`);
      setDeleteDialogOpen(false);
      setUserToDelete(null);
      fetchAdminData(); // Refresh user list
    } catch (err: any) {
      console.error('Failed to delete user:', err);
      setDeleteError(err.response?.data || 'Failed to delete user. Please try again.');
    }
  };

  const resetPassword = async (userId: number) => {
    try {
      await api.post(`/api/admin/users/${userId}/reset-password`);
      alert('Password reset email sent');
    } catch (err) {
      console.error('Failed to reset password:', err);
      alert('Failed to reset password');
    }
  };

  const triggerScan = async () => {
    setScanning(true);
    setScanMessage('');
    try {
      await api.post('/api/admin/scan');
      setScanMessage('Scan started. It runs in the background and may take a few minutes.');
    } catch (err) {
      console.error('Failed to trigger scan:', err);
      setScanMessage('Failed to start scan.');
    } finally {
      setScanning(false);
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
        <h1 className="text-3xl font-bold mb-8">Admin Dashboard</h1>

        {/* Users Section */}
        <Card className="mb-6">
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Users</CardTitle>
              <CardDescription>Manage user accounts and approvals</CardDescription>
            </div>
            <Button onClick={openCreateUserDialog}>Create New User</Button>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {users.map(user => (
                <div key={user.id} className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 sm:gap-4 p-4 border rounded-lg">
                  <div className="flex-1 min-w-0">
                    <p className="font-medium">{user.name} ({user.email})</p>
                    <p className="text-sm text-muted-foreground">Role: {user.role}</p>
                  </div>
                  <div className="flex flex-wrap items-center gap-3">
                    <div className="flex items-center gap-2">
                      <Checkbox
                        id={`admin-${user.id}`}
                        checked={user.role === 'admin'}
                        onCheckedChange={() => toggleAdmin(user.id)}
                      />
                      <label
                        htmlFor={`admin-${user.id}`}
                        className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                      >
                        Admin
                      </label>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button variant="outline" size="sm" onClick={() => openPasswordDialog(user.id)}>
                        Change Password
                      </Button>
                      <Button variant="outline" size="sm" onClick={() => resetPassword(user.id)}>
                        Reset Password
                      </Button>
                      {!user.approved && (
                        <Button onClick={() => approveUser(user.id)} size="sm">
                          Approve
                        </Button>
                      )}
                      <Button variant="destructive" size="sm" onClick={() => openDeleteDialog(user.id)}>
                        Delete
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Account Requests Section */}
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Account Requests</CardTitle>
            <CardDescription>Pending requests for new user accounts</CardDescription>
          </CardHeader>
          <CardContent>
            {accountRequests.length === 0 ? (
              <p className="text-muted-foreground">No pending account requests.</p>
            ) : (
              <div className="space-y-4">
                {accountRequests.map(request => (
                  <div key={request.id} className="p-4 border rounded-lg">
                    <div className="flex items-center justify-between mb-2">
                      <div>
                        <p className="font-medium">{request.name}</p>
                        <p className="text-sm text-muted-foreground">{request.email}</p>
                      </div>
                      <Badge variant={request.status === "pending" ? "secondary" : "default"}>
                        {request.status}
                      </Badge>
                    </div>
                    {request.message && (
                      <p className="text-sm mb-3 p-2 bg-muted rounded">{request.message}</p>
                    )}
                    {request.status === "pending" && (
                      <div className="flex gap-2">
                        <Button variant="default" size="sm" onClick={() => reviewAccountRequest(request.id, "approved")}>
                          Approve
                        </Button>
                        <Button variant="destructive" size="sm" onClick={() => reviewAccountRequest(request.id, "rejected")}>
                          Reject
                        </Button>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Proposed Edits Section */}
        <Card>
          <CardHeader>
            <CardTitle>Proposed Edits</CardTitle>
            <CardDescription>Review and approve/reject user-suggested changes</CardDescription>
          </CardHeader>
          <CardContent>
            {proposedEdits.length === 0 ? (
              <p className="text-muted-foreground">No proposed edits pending review.</p>
            ) : (
              <div className="space-y-4">
                {proposedEdits.map(edit => (
                  <div key={edit.id} className="p-4 border rounded-lg">
                    <div className="flex items-center justify-between mb-2">
                      <div>
                        <p className="font-medium">Photo #{edit.photo_id}</p>
                        <p className="text-sm text-muted-foreground">Field: {edit.field}</p>
                      </div>
                      <Badge variant={edit.status === "pending" ? "secondary" : "default"}>
                        {edit.status}
                      </Badge>
                    </div>
                    <div className="grid grid-cols-2 gap-4 text-sm mb-4">
                      <div>
                        <p className="text-muted-foreground">Current Value:</p>
                        <p className="bg-muted p-2 rounded">{edit.current_value || "(empty)"}</p>
                      </div>
                      <div>
                        <p className="text-muted-foreground">Proposed Value:</p>
                        <p className="bg-muted p-2 rounded">{edit.proposed_value}</p>
                      </div>
                    </div>
                    <div className="flex items-center justify-between">
                      <p className="text-sm text-muted-foreground">
                        Submitted by: {edit.user_email}
                      </p>
                      {edit.status === "pending" && (
                        <div className="flex gap-2">
                          <Button variant="default" size="sm" onClick={() => reviewProposedEdit(edit.id, "approved")}>
                            Approve
                          </Button>
                          <Button variant="destructive" size="sm" onClick={() => reviewProposedEdit(edit.id, "rejected")}>
                            Reject
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Tags Section */}
        <Card className="mb-6">
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Tags</CardTitle>
              <CardDescription>Manage photo tags - create, edit, or delete tags</CardDescription>
            </div>
            <Link to="/admin/tags">
              <Button>
                <Settings className="h-4 w-4 mr-2" />
                Manage Tags
                <ExternalLink className="h-4 w-4 ml-2" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            <p className="text-muted-foreground text-sm">
              Tags allow you to categorize and mark photos with labels like "Family", "Vacation", etc.
            </p>
          </CardContent>
        </Card>

        {/* Photo Scanner Section */}
        <Card className="mb-6">
          <CardHeader>
            <CardTitle>Photo Scanner</CardTitle>
            <CardDescription>Scan photo directories for new, changed, or deleted files</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground mb-4">
              Scans all configured photo directories, adds new photos, updates changed ones,
              and removes database records for files that no longer exist on disk.
            </p>
            <div className="flex items-center gap-4">
              <Button onClick={triggerScan} disabled={scanning}>
                {scanning ? 'Scanning...' : 'Trigger Scan'}
              </Button>
              {scanMessage && (
                <p className="text-sm text-muted-foreground">{scanMessage}</p>
              )}
            </div>
          </CardContent>
        </Card>

        {/* Change Password Dialog */}
        <Dialog open={passwordDialogOpen} onOpenChange={setPasswordDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Change User Password</DialogTitle>
              <DialogDescription>
                Enter a new password for the selected user.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <Input
                  type="password"
                  placeholder="New password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  autoFocus
                />
                <Input
                  type="password"
                  placeholder="Confirm password"
                  value={confirmNewPassword}
                  onChange={(e) => setConfirmNewPassword(e.target.value)}
                />
              </div>
              {passwordError && (
                <p className="text-sm text-red-500">{passwordError}</p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setPasswordDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={changePassword}>
                Change Password
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Create New User Dialog */}
        <Dialog open={createUserDialogOpen} onOpenChange={setCreateUserDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create New User</DialogTitle>
              <DialogDescription>
                Enter the details for the new user account.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <Input
                  placeholder="First name"
                  value={newUser.firstName}
                  onChange={(e) => setNewUser({ ...newUser, firstName: e.target.value })}
                  autoFocus
                />
                <Input
                  placeholder="Last name"
                  value={newUser.lastName}
                  onChange={(e) => setNewUser({ ...newUser, lastName: e.target.value })}
                />
              </div>
              <Input
                type="email"
                placeholder="Email address"
                value={newUser.email}
                onChange={(e) => setNewUser({ ...newUser, email: e.target.value })}
              />
              <div className="grid grid-cols-2 gap-4">
                <Input
                  type="password"
                  placeholder="Password (leave empty for reset link)"
                  value={newUser.password}
                  onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
                />
                <Input
                  type="password"
                  placeholder="Confirm password"
                  value={newUser.confirmPassword}
                  onChange={(e) => setNewUser({ ...newUser, confirmPassword: e.target.value })}
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Leave password fields empty to send a password reset link via email
              </p>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="create-admin"
                  checked={newUser.isAdmin}
                  onCheckedChange={(checked) => setNewUser({ ...newUser, isAdmin: !!checked })}
                />
                <label
                  htmlFor="create-admin"
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                >
                  Admin user
                </label>
              </div>
              {createUserError && (
                <p className="text-sm text-red-500">{createUserError}</p>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateUserDialogOpen(false)}>
                Cancel
              </Button>
              <Button onClick={createUser}>
                Create User
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Delete User Confirmation Dialog */}
        <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Delete User</DialogTitle>
              <DialogDescription>
                Are you sure you want to delete this user? This action cannot be undone.
              </DialogDescription>
            </DialogHeader>
            {deleteError && (
              <p className="text-sm text-red-500">{deleteError}</p>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleteDialogOpen(false)}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={deleteUser}>
                Delete User
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>
  );
}