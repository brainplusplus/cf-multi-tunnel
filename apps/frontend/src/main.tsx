import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { ToastProvider } from './components/Toast';
import { AuthProvider } from './store/auth-context';
import { LanguageProvider } from './i18n/LanguageContext';
import './index.css';

// The router and the three context providers must wrap <App />. App renders
// <Routes>, which throws "useRoutes() may be used only in the context of a
// <Router> component" without BrowserRouter, and every page calls useAuth(),
// useToast() or useTranslation(), which throw without their providers. Result
// without this tree: a blank page and one console error.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <LanguageProvider>
        <ToastProvider>
          <AuthProvider>
            <App />
          </AuthProvider>
        </ToastProvider>
      </LanguageProvider>
    </BrowserRouter>
  </StrictMode>,
);
