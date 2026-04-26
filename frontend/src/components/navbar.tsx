import { Link } from 'react-router-dom';
import { ThemeToggle } from './theme-toggle';
import { User } from 'lucide-react';
import { Button } from './ui/button';

export function Navbar() {
  return (
    <nav className="flex items-center justify-between border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 px-8 py-4">
      <div className="flex items-center gap-6">
        <Link to="/" className="text-xl font-bold bg-gradient-to-r from-primary to-purple-600 bg-clip-text text-transparent">
          MoopicView
        </Link>
        <div className="flex items-center gap-4 text-sm">
          <Link to="/collections" className="text-muted-foreground hover:text-foreground transition-colors">
            Collections
          </Link>
          <Link to="/account" className="text-muted-foreground hover:text-foreground transition-colors">
            Account
          </Link>
          <Link to="/admin" className="text-muted-foreground hover:text-foreground transition-colors">
            Admin
          </Link>
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
      </div>
    </nav>
  );
}
