import { Box, CircularProgress, Typography } from '@mui/material';
import { Suspense, lazy, type FC, type ReactNode, useEffect } from 'react';
import { useDispatch } from 'react-redux';
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom';
import Layout from '@/components/Layout';
import ProtectedRoute from '@/components/ProtectedRoute';
import { ToastProvider } from '@/services/toast';
import { resetStore, type AppDispatch } from '@/store';
import { clearAuth } from '@/store/authSlice';

// ---------------------------------------------------------------------------
// Lazy-loaded page components for code splitting
// ---------------------------------------------------------------------------

const DashboardPage = lazy(() => import('@/pages/DashboardPage'));
const ExperimentListPage = lazy(() => import('@/pages/ExperimentListPage'));
const CreateExperimentPage = lazy(() => import('@/pages/CreateExperimentPage'));
const ExperimentDetailPage = lazy(() => import('@/pages/ExperimentDetailPage'));
const ClusterListPage = lazy(() => import('@/pages/ClusterListPage'));
const TemplateListPage = lazy(() => import('@/pages/TemplateListPage'));
const ReportsPage = lazy(() => import('@/pages/ReportsPage'));
const SettingsPage = lazy(() => import('@/pages/SettingsPage'));
const LoginPage = lazy(() => import('@/pages/LoginPage'));
const RegisterPage = lazy(() => import('@/pages/RegisterPage'));

// ---------------------------------------------------------------------------
// Loading Fallback
// ---------------------------------------------------------------------------

const PageLoader: FC = () => (
  <Box
    sx={{
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      height: '100%',
      minHeight: '50vh',
      gap: 2,
    }}
  >
    <CircularProgress size={36} thickness={4} />
    <Typography variant="body2" color="text.secondary">
      Loading...
    </Typography>
  </Box>
);

// ---------------------------------------------------------------------------
// Suspense Wrapper for Lazy Pages
// ---------------------------------------------------------------------------

const LazyPage: FC<{ children: ReactNode }> = ({ children }) => (
  <Suspense fallback={<PageLoader />}>{children}</Suspense>
);

// ---------------------------------------------------------------------------
// Auth session watcher
// ---------------------------------------------------------------------------

const AuthSessionWatcher: FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const location = useLocation();
  const navigate = useNavigate();

  useEffect(() => {
    const handleAuthExpired = (event: Event) => {
      const detail = (event as CustomEvent<{ reason?: string }>).detail;
      dispatch(clearAuth());
      dispatch(resetStore());

      const redirectTo = encodeURIComponent(location.pathname + location.search);
      const loginUrl = `/login?redirect=${redirectTo}&expired=1`;

      if (location.pathname !== '/login') {
        navigate(loginUrl, { replace: true });
      }

      if (process.env.NODE_ENV !== 'production') {
        console.warn('[Chaos-Sec] auth session expired', detail?.reason ?? 'unknown');
      }
    };

    window.addEventListener('chaos-sec:auth-expired', handleAuthExpired);
    return () => window.removeEventListener('chaos-sec:auth-expired', handleAuthExpired);
  }, [dispatch, location.pathname, location.search, navigate]);

  return null;
};

// ---------------------------------------------------------------------------
// App Component
// ---------------------------------------------------------------------------

const App: FC = () => {
  return (
    <ToastProvider defaultPosition="bottom-right" maxToasts={5}>
      <AuthSessionWatcher />
      <Routes>
        {/* ----------------------------------------------------------------- */}
        {/* Public Routes – no authentication required                        */}
        {/* ----------------------------------------------------------------- */}
        <Route
          path="/login"
          element={
            <LazyPage>
              <LoginPage />
            </LazyPage>
          }
        />
        <Route
          path="/register"
          element={
            <LazyPage>
              <RegisterPage />
            </LazyPage>
          }
        />

        {/* ----------------------------------------------------------------- */}
        {/* Protected Routes – authentication required                         */}
        {/* Wrapped in Layout for sidebar + navbar                             */}
        {/* ----------------------------------------------------------------- */}
        <Route
          path="/"
          element={
            <ProtectedRoute>
              <Layout />
            </ProtectedRoute>
          }
        >
          {/* Dashboard */}
          <Route
            index
            element={
              <LazyPage>
                <DashboardPage />
              </LazyPage>
            }
          />

          {/* Experiments */}
          <Route
            path="experiments"
            element={
              <LazyPage>
                <ExperimentListPage />
              </LazyPage>
            }
          />
          <Route
            path="experiments/new"
            element={
              <LazyPage>
                <CreateExperimentPage />
              </LazyPage>
            }
          />
          <Route
            path="experiments/:id"
            element={
              <LazyPage>
                <ExperimentDetailPage />
              </LazyPage>
            }
          />

          {/* Clusters */}
          <Route
            path="clusters"
            element={
              <LazyPage>
                <ClusterListPage />
              </LazyPage>
            }
          />

          {/* Templates */}
          <Route
            path="templates"
            element={
              <LazyPage>
                <TemplateListPage />
              </LazyPage>
            }
          />

          {/* Reports */}
          <Route
            path="reports"
            element={
              <LazyPage>
                <ReportsPage />
              </LazyPage>
            }
          />

          {/* Settings */}
          <Route
            path="settings"
            element={
              <LazyPage>
                <SettingsPage />
              </LazyPage>
            }
          />
        </Route>

        {/* ----------------------------------------------------------------- */}
        {/* Catch-all – redirect unknown routes to dashboard                  */}
        {/* ----------------------------------------------------------------- */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ToastProvider>
  );
};

export default App;
