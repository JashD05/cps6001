import React, { useEffect, useState } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import { Box, CircularProgress, Typography } from '@mui/material';
import { Security } from '@mui/icons-material';
import { RootState, AppDispatch } from '@/store';
import { clearAuth, me, setAuthFromStorage } from '@/store/authSlice';
import { getAccessToken, getRefreshToken } from '@/services/api';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const dispatch = useDispatch<AppDispatch>();
  const location = useLocation();
  const authState = useSelector((state: RootState) => state.auth) as RootState['auth'] & {
    loading?: string;
  };
  const isAuthenticated = authState.isAuthenticated;
  const isLoading = authState.isLoading ?? authState.loading === 'pending';
  const [isCheckingSession, setIsCheckingSession] = useState(true);
  const [hadTokens, setHadTokens] = useState(false);

  // On mount, restore auth state from stored tokens or redirect immediately if none exist.
  useEffect(() => {
    if (isAuthenticated) {
      setIsCheckingSession(false);
      return;
    }

    if (isLoading) {
      return;
    }

    const accessToken = getAccessToken();
    const refreshToken = getRefreshToken();

    if (accessToken && refreshToken) {
      setHadTokens(true);
      dispatch(setAuthFromStorage({ accessToken, refreshToken }));
      Promise.resolve(dispatch(me())).finally(() => setIsCheckingSession(false));
      return;
    }

    dispatch(clearAuth());
    setIsCheckingSession(false);
  }, [dispatch, isAuthenticated, isLoading]);

  // Still loading or checking — show a spinner while we verify the session
  if (isCheckingSession || (isLoading && !isAuthenticated)) {
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
    const redirectUrl = hadTokens
      ? `/login?redirect=${redirectTo}&expired=1`
      : `/login?redirect=${redirectTo}`;
    return <Navigate to={redirectUrl} replace />;
  }

  // Authenticated — render the protected content
  return <>{children}</>;
};

export default ProtectedRoute;
