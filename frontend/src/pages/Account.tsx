import { useState } from 'react';
import { useAuth } from '../hooks/useAuth';
import { Link } from 'react-router-dom';
import axios from 'axios';
import { Save, LogOut, ArrowLeft } from 'lucide-react';

export default function Account() {
  const { logout } = useAuth();
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [message, setMessage] = useState('');

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setMessage('');
    try {
      const token = localStorage.getItem('token');
      await axios.post('/api/auth/change-password', {
        oldPassword,
        newPassword,
      }, {
        headers: { Authorization: `Bearer ${token}` },
      });
      setMessage('Password updated successfully.');
      setOldPassword('');
      setNewPassword('');
    } catch (err: any) {
      setMessage('Error: ' + (err.response?.data || 'Failed to update password'));
    }
    setIsLoading(false);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-white p-8">
      <div className="max-w-md mx-auto">
        <div className="flex justify-between items-center mb-8">
          <Link to="/browse" className="flex items-center gap-2 text-zinc-400 hover:text-white">
            <ArrowLeft className="w-5 h-5" /> Back to Collections
          </Link>
          <button
            onClick={logout}
            className="flex items-center gap-2 text-red-400 hover:text-red-500"
          >
            <LogOut className="w-5 h-5" /> Logout
          </button>
        </div>
        <h1 className="text-3xl font-bold mb-8">Account Settings</h1>

        <div className="bg-zinc-900 rounded-2xl p-8">
          <form onSubmit={handleChangePassword} className="space-y-6">
            <div>
              <label className="block text-sm text-zinc-400 mb-2">Current Password</label>
              <input
                type="password"
                value={oldPassword}
                onChange={(e) => setOldPassword(e.target.value)}
                className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white"
                required
              />
            </div>
            <div>
              <label className="block text-sm text-zinc-400 mb-2">New Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white"
                required
              />
            </div>
            <button
              type="submit"
              disabled={isLoading}
              className="w-full bg-emerald-600 hover:bg-emerald-700 py-3 rounded-lg font-medium flex items-center justify-center gap-2 disabled:opacity-70"
            >
              <Save className="w-5 h-5" />
              {isLoading ? 'Updating...' : 'Update Password'}
            </button>
          </form>
          {message && <div className="mt-4 p-3 rounded-lg bg-zinc-800 text-sm">{message}</div>}
        </div>
      </div>
    </div>
  );
}
