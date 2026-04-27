import React, { useEffect } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import { Box, CircularProgress, Typography } from '@mui/material';
import { Security } from '@mui/icons-material';
import { RootState, AppDispatch } from '@/store';
import { me, setAuthFromStorage } from '@/store/authSlice';
import { getAccessToken, getRefreshToken } from '@/services/api';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const dispatch = useDispatch<AppDispatch>();
  const location = useLocation();
  const { isAuthenticated, isLoading, user } = useSelector(
    (state: RootState) => state.auth,
  );

  // On mount, try to restore auth state from stored tokens
  useEffect(() => {
    const accessToken = getAccessToken();
    const refreshToken = getRefreshToken();
    if (accessToken && refreshToken && !isAuthenticated && !isLoading) {
      // We have tokens but Redux says we're not authenticated — try to restore
      dispatch(setAuthFromStorage({ accessToken, refreshToken }));
      dispatch(me());
    }
  }, [dispatch, isAuthenticated, isLoading]);

  // Still loading — show a spinner while we verify the session
  if (isLoading && !isAuthenticated) {
    return (
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 2,
        }}
      >
        <Security sx={{ fontSize: 48, color: 'primary.main', opacity: 0.6 }} />
        <CircularProgress size={32} />
        <Typography variant="body2" color="text.secondary">
          Verifying session…
        </Typography>
      </Box>
    );
  }

  // Not authenticated — redirect to login, preserving the intended destination
  if (!isAuthenticated) {
    const redirectTo = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?redirect=${redirectTo}&expired=1`} replace />;
  }

  // Authenticated — render the protected content
  return <>{children}</>;
};

export default ProtectedRoute;
