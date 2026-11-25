import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { ToastProvider } from './components/Toast';
import { AuthProvider } from './store/auth-context';
import './index.css';

// The router and the two context providers must wrap <App />. App renders
// <Routes>, which throws "useRoutes() may be used only in the context of a
// <Router> component" without BrowserRouter, and every page calls useAuth()
// and useToast(), which throw without their providers. Result without this
// tree: a blank page and one console error.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <ToastProvider>
        <AuthProvider>
          <App />
        </AuthProvider>
      </ToastProvider>
    </BrowserRouter>
  </StrictMode>,
);
