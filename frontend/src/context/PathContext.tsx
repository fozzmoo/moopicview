import { createContext, useState, useContext, type ReactNode, type Dispatch, type SetStateAction } from 'react';

type PathItem = { path: string; name: string };

type PathContextType = {
  pathStack: PathItem[];
  currentPath: string;
  setPathStack: Dispatch<SetStateAction<PathItem[]>>;
  setCurrentPath: Dispatch<SetStateAction<string>>;
  addToPathStack: (item: PathItem) => void;
  goBackInPath: () => void;
  resetPath: () => void;
};

const PathContext = createContext<PathContextType | undefined>(undefined);

export const PathProvider = ({ children }: { children: ReactNode }) => {
  const [pathStack, setPathStack] = useState<PathItem[]>([]);
  const [currentPath, setCurrentPath] = useState<string>('');

  const addToPathStack = (item: PathItem) => {
    setPathStack((prevStack) => [...prevStack, item]);
  };

  const goBackInPath = () => {
    setPathStack((prevStack) => prevStack.slice(0, -1));
  };

  const resetPath = () => {
    setPathStack([]);
    setCurrentPath('');
  };

  return (
    <PathContext.Provider value={{ pathStack, currentPath, setPathStack, setCurrentPath, addToPathStack, goBackInPath, resetPath }}>
      {children}
    </PathContext.Provider>
  );
};

export const usePath = () => {
  const context = useContext(PathContext);
  if (context === undefined) {
    throw new Error('usePath must be used within a PathProvider');
  }
  return context;
};
