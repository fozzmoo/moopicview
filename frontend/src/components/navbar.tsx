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
    <nav className="relative flex items-center justify-between border-b px-4 md:px-8 py-4" style={{ backgroundColor: 'hsl(var(--color-background))' }}>
      <div className="flex items-center gap-6">
        <Link to="/collections?reset=true" className="text-xl font-bold" style={{ color: '#b30074' }}>
          MooView
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
          aria-label={mobileMenuOpen ? 'Close menu' : 'Open menu'}
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
        <div
          data-testid="mobile-dropdown-menu"
          className="absolute top-full right-0 md:hidden border-b shadow-lg z-50"
          style={{ backgroundColor: 'hsl(var(--color-background))' }}
        >
          <div className="flex flex-col px-4 py-3 gap-2 text-right">
            <Link 
              to="/collections?reset=true" 
              data-testid="mobile-collections-link"
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              style={{ color: 'hsl(var(--color-muted-foreground))' }}
              onClick={() => setMobileMenuOpen(false)}
            >
              Collections
            </Link>
            <Link 
              to="/tags" 
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              style={{ color: 'hsl(var(--color-muted-foreground))' }}
              onClick={() => setMobileMenuOpen(false)}
            >
              Tags
            </Link>
            <Link 
              to="/account" 
              className="text-muted-foreground hover:text-foreground transition-colors py-2"
              style={{ color: 'hsl(var(--color-muted-foreground))' }}
              onClick={() => setMobileMenuOpen(false)}
            >
              Account
            </Link>
            {isAdmin && (
              <Link 
                to="/admin" 
                className="text-muted-foreground hover:text-foreground transition-colors py-2"
                style={{ color: 'hsl(var(--color-muted-foreground))' }}
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
