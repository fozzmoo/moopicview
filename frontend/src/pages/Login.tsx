import { useState } from 'react';
import { useAuth } from '../hooks/useAuth';
import { useNavigate } from 'react-router-dom';
import { LogIn, UserPlus } from 'lucide-react';

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<'signin' | 'request'>('signin');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [requestMessage, setRequestMessage] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError('');
    setSuccess('');
    try {
      await login(email, password);
      navigate('/collections');
    } catch (err: any) {
      console.error('Login error:', err);
      setError(err.response?.data || 'Login failed. Please check your credentials.');
    }
    setIsLoading(false);
  };

  const handleRequestAccess = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError('');
    setSuccess('');
    try {
      const response = await fetch('/api/auth/request-access', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, name, message: requestMessage }),
      });
      if (!response.ok) {
        throw new Error(await response.text());
      }
      setSuccess('Request submitted! You will be contacted when your account is approved.');
      setEmail('');
      setName('');
      setRequestMessage('');
    } catch (err: any) {
      console.error('Request access error:', err);
      setError(err.message || 'Failed to submit request.');
    }
    setIsLoading(false);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-white flex items-center justify-center">
      <div className="w-full max-w-md p-8">
        <div className="text-center mb-10">
          <h1 className="text-4xl font-bold tracking-tight mb-2">moopicview</h1>
          <p className="text-zinc-400">Private photo collections</p>
        </div>

        <div className="bg-zinc-900 rounded-2xl p-8 shadow-xl">
          {/* Tab Navigation */}
          <div className="flex mb-6 bg-zinc-800 rounded-lg p-1">
            <button
              onClick={() => setActiveTab('signin')}
              className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                activeTab === 'signin'
                  ? 'bg-violet-600 text-white'
                  : 'text-zinc-400 hover:text-white'
              }`}
            >
              Sign In
            </button>
            <button
              onClick={() => setActiveTab('request')}
              className={`flex-1 py-2 px-4 rounded-md text-sm font-medium transition-colors ${
                activeTab === 'request'
                  ? 'bg-violet-600 text-white'
                  : 'text-zinc-400 hover:text-white'
              }`}
            >
              Request Access
            </button>
          </div>

          {/* Sign In Form */}
          {activeTab === 'signin' && (
            <form onSubmit={handleLogin} className="space-y-6">
              <div>
                <label htmlFor="email" className="block text-sm text-zinc-400 mb-2">Email</label>
                <input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                  required
                />
              </div>
              <div>
                <label htmlFor="password" className="block text-sm text-zinc-400 mb-2">Password</label>
                <input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                  required
                />
              </div>
              {error && <div className="text-red-400 text-sm p-3 bg-red-950/50 rounded-lg">{error}</div>}
              {success && <div className="text-green-400 text-sm p-3 bg-green-950/50 rounded-lg">{success}</div>}
              <button
                type="submit"
                disabled={isLoading}
                className="w-full bg-violet-600 hover:bg-violet-700 py-3 rounded-lg font-medium flex items-center justify-center gap-2 transition-colors disabled:opacity-70"
              >
                <LogIn className="w-5 h-5" />
                {isLoading ? 'Signing in...' : 'Sign in'}
              </button>
              <div className="text-center text-sm text-zinc-500">
                <a href="/api/auth/google" className="hover:text-violet-400">
                  Sign in with Google
                </a>
                <p className="text-xs text-zinc-600 mt-1">
                  (Requires Google OAuth configuration)
                </p>
              </div>
            </form>
          )}

          {/* Request Access Form */}
          {activeTab === 'request' && (
            <form onSubmit={handleRequestAccess} className="space-y-6">
              <div>
                <label htmlFor="request-email" className="block text-sm text-zinc-400 mb-2">Email *</label>
                <input
                  id="request-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                  required
                />
              </div>
              <div>
                <label htmlFor="request-name" className="block text-sm text-zinc-400 mb-2">Name *</label>
                <input
                  id="request-name"
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                  required
                />
              </div>
              <div>
                <label htmlFor="request-message" className="block text-sm text-zinc-400 mb-2">Message (optional)</label>
                <textarea
                  id="request-message"
                  value={requestMessage}
                  onChange={(e) => setRequestMessage(e.target.value)}
                  rows={3}
                  className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500 resize-none"
                />
              </div>
              {error && <div className="text-red-400 text-sm p-3 bg-red-950/50 rounded-lg">{error}</div>}
              {success && <div className="text-green-400 text-sm p-3 bg-green-950/50 rounded-lg">{success}</div>}
              <button
                type="submit"
                disabled={isLoading}
                className="w-full bg-violet-600 hover:bg-violet-700 py-3 rounded-lg font-medium flex items-center justify-center gap-2 transition-colors disabled:opacity-70"
              >
                <UserPlus className="w-5 h-5" />
                {isLoading ? 'Submitting...' : 'Request Access'}
              </button>
            </form>
          )}
        </div>

        <div className="text-center mt-8 text-xs text-zinc-500">
          Admin access available after approval
        </div>
      </div>
    </div>
  );
}
