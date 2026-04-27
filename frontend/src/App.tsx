import React, { Suspense, type ReactNode } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Box, CircularProgress, Typography } from '@mui/material';
import Layout from '@/components/Layout';
import ProtectedRoute from '@/components/ProtectedRoute';
import { ToastProvider } from '@/services/toast';

// ---------------------------------------------------------------------------
// Lazy-loaded page components for code splitting
// ---------------------------------------------------------------------------

const DashboardPage = React.lazy(() => import('@/pages/DashboardPage'));
const ExperimentListPage = React.lazy(() => import('@/pages/ExperimentListPage'));
const CreateExperimentPage = React.lazy(() => import('@/pages/CreateExperimentPage'));
const ExperimentDetailPage = React.lazy(() => import('@/pages/ExperimentDetailPage'));
const ClusterListPage = React.lazy(() => import('@/pages/ClusterListPage'));
const TemplateListPage = React.lazy(() => import('@/pages/TemplateListPage'));
const ReportsPage = React.lazy(() => import('@/pages/ReportsPage'));
const SettingsPage = React.lazy(() => import('@/pages/SettingsPage'));
const LoginPage = React.lazy(() => import('@/pages/LoginPage'));
const RegisterPage = React.lazy(() => import('@/pages/RegisterPage'));

// ---------------------------------------------------------------------------
// Loading Fallback
// ---------------------------------------------------------------------------

const PageLoader: React.FC = () => (
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

const LazyPage: React.FC<{ children: ReactNode }> = ({ children }) => (
  <Suspense fallback={<PageLoader />}>{children}</Suspense>
);

// ---------------------------------------------------------------------------
// App Component
// ---------------------------------------------------------------------------

const App: React.FC = () => {
  return (
    <ToastProvider defaultPosition="bottom-right" maxToasts={5}>
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
