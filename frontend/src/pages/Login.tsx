import { useState } from 'react';
import { useAuth } from '../hooks/useAuth';
import { useNavigate } from 'react-router-dom';
import { LogIn } from 'lucide-react';

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');

  const handleLogin = async (e: React.FormEvent) => {
    console.log('Login submitted with', email);
    e.preventDefault();
    setIsLoading(true);
    setError('');
    try {
      await login(email, password);
      navigate('/browse');
    } catch (err: any) {
      console.error('Login error:', err);
      setError(err.response?.data || 'Login failed. Try admin@fozzilinymoo.org / admin123');
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
          <form onSubmit={handleLogin} className="space-y-6">
            <div>
              <label className="block text-sm text-zinc-400 mb-2">Email</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                required
              />
            </div>
            <div>
              <label className="block text-sm text-zinc-400 mb-2">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-3 text-white focus:outline-none focus:border-violet-500"
                required
              />
            </div>
            {error && <div className="text-red-400 text-sm p-3 bg-red-950/50 rounded-lg">{error}</div>}
            <button
              type="submit"
              disabled={isLoading}
              className="w-full bg-violet-600 hover:bg-violet-700 py-3 rounded-lg font-medium flex items-center justify-center gap-2 transition-colors disabled:opacity-70"
            >
              <LogIn className="w-5 h-5" />
              {isLoading ? 'Signing in...' : 'Sign in'}
            </button>
          </form>

          <div className="mt-6 text-center text-sm text-zinc-500">
            or sign in with Google • Request access
          </div>
        </div>

        <div className="text-center mt-8 text-xs text-zinc-500">
          Admin access available after approval
        </div>
      </div>
    </div>
  );
}
