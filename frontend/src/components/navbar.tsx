import { Link } from 'react-router-dom';
import { ThemeToggle } from './theme-toggle';
import { User, Menu, X } from 'lucide-react';
import { Button } from './ui/button';
import { useAuth } from '@/hooks/useAuth';
import { useState } from 'react';

export function Navbar() {
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <nav className="flex items-center justify-between border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 px-4 md:px-8 py-4">
      <div className="flex items-center gap-6">
        <Link to="/collections?reset=true" className="text-xl font-bold bg-gradient-to-r from-primary to-purple-600 bg-clip-text text-transparent">
          MoopicView
        </Link>
        {/* Desktop nav links */}
        <div className="hidden md:flex items-center gap-4 text-sm">
          <Link to="/collections?reset=true" className="text-muted-foreground hover:text-foreground transition-colors">
            Collections
          </Link>
          <Link to="/tags" className="text-muted-foreground hover:text-foreground transition-colors">
            Tags
          </Link>
          <Link to="/account" className="text-muted-foreground hover:text-foreground transition-colors">
            Account
          </Link>
          {isAdmin && (
            <Link to="/admin" className="text-muted-foreground hover:text-foreground transition-colors">
              Admin
            </Link>
          )}
        </div>
      </div>
      <div className="flex items-center gap-4">
        <ThemeToggle />
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" asChild>
            <Link to="/account">
              <User className="h-4 w-4 text-foreground" />
            </Link>
          </Button>
        </div>
        {/* Mobile menu button */}
        <Button 
          variant="ghost" 
          size="icon" 
          className="md:hidden"
          onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
        >
          {mobileMenuOpen ? (
            <X className="h-5 w-5 text-foreground" />
          ) : (
            <Menu className="h-5 w-5 text-foreground" />
          )}
        </Button>
      </div>
      {/* Mobile dropdown menu */}
      {mobileMenuOpen && (
        <div className="absolute top-full left-0 right-0 md:hidden bg-background border-b shadow-lg z-50">
          <div className="flex flex-col px-4 py-3 gap-2">
            <Link 
              to="/collections?reset=true" 
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              onClick={() => setMobileMenuOpen(false)}
            >
              Collections
            </Link>
            <Link 
              to="/tags" 
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              onClick={() => setMobileMenuOpen(false)}
            >
              Tags
            </Link>
            <Link 
              to="/account" 
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              onClick={() => setMobileMenuOpen(false)}
            >
              Account
            </Link>
            {isAdmin && (
              <Link 
                to="/admin" 
                className="text-muted-foreground hover:text-foreground transition-colors py-2"
                onClick={() => setMobileMenuOpen(false)}
              >
                Admin
              </Link>
            )}
          </div>
        </div>
      )}
    </nav>
  );
}
