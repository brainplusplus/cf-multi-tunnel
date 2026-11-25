import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

export type Language = 'id' | 'en';

interface LanguageContextValue {
  language: Language;
  setLanguage: (lang: Language) => void;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

const STORAGE_KEY = 'language';

// Indonesian is the default because the operators of this panel are Indonesian.
// An unknown or missing value falls back to it rather than to English.
function initialLanguage(): Language {
  const saved = localStorage.getItem(STORAGE_KEY);
  return saved === 'en' || saved === 'id' ? saved : 'id';
}

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>(initialLanguage);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, language);
    // Screen readers and the browser's own spell-check use this.
    document.documentElement.lang = language;
  }, [language]);

  const value = useMemo(() => ({ language, setLanguage }), [language]);
  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

export function useLanguage() {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error('useLanguage must be used inside LanguageProvider');
  return ctx;
}
