import { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import api from '@/lib/api';

interface AuthContextType {
  isAuthenticated: boolean;
  user: any | null;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [user, setUser] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (token) {
      try {
        // Decode JWT payload (base64url encoded)
        const payload = token.split('.')[1];
        const decodedJson = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
        const decoded = JSON.parse(decodedJson);
        
        setIsAuthenticated(true);
        setUser({ 
          email: decoded.email, 
          role: decoded.role 
        });
      } catch (error) {
        console.error("Failed to decode token:", error);
        localStorage.removeItem('token');
        setIsAuthenticated(false);
        setUser(null);
      }
    }
    setLoading(false);
  }, []);

  const login = async (email: string, password: string) => {
    const res = await api.post('/api/auth/login', { email, password });
    const token = res.data.token;
    localStorage.setItem('token', token);
    setIsAuthenticated(true);
    
    // Decode the new token to get user details
    try {
      const payload = token.split('.')[1];
      const decodedJson = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
      const decoded = JSON.parse(decodedJson);
      setUser({ 
        email: decoded.email, 
        role: decoded.role 
      });
    } catch (error) {
      console.error("Failed to decode login token:", error);
      setUser({ email, role: 'user' }); // Fallback
    }
  };

  const logout = () => {
    localStorage.removeItem('token');
    setIsAuthenticated(false);
    setUser(null);
  };

  if (loading) return null;

  return (
    <AuthContext.Provider value={{ isAuthenticated, user, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
};
