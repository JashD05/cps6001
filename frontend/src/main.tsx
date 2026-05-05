import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider, StyledEngineProvider } from '@mui/material/styles';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Provider } from 'react-redux';
import { BrowserRouter } from 'react-router-dom';
import App from '@/App';
import { store } from '@/store';
import { lightTheme } from '@/theme';

// ---------------------------------------------------------------------------
// Keep stored auth tokens so users stay signed in across page refreshes.
// ---------------------------------------------------------------------------
// Global error handler for unhandled promise rejections
// ---------------------------------------------------------------------------

window.addEventListener('unhandledrejection', (event) => {
  // Prevent default browser behavior for unhandled promise rejections
  event.preventDefault();

  if (import.meta.env.DEV) {
    console.error('Unhandled promise rejection:', event.reason);
  }
});

// ---------------------------------------------------------------------------
// Mount the application
// ---------------------------------------------------------------------------

const rootElement = document.getElementById('root');

if (!rootElement) {
  throw new Error(
    'Root element not found. Make sure there is a <div id="root"> in your index.html.',
  );
}

const root = createRoot(rootElement);

root.render(
  <StrictMode>
    <Provider store={store}>
      <StyledEngineProvider injectFirst>
        <ThemeProvider theme={lightTheme}>
          <CssBaseline enableColorScheme />
          <LocalizationProvider dateAdapter={AdapterDateFns}>
            <BrowserRouter>
              <App />
            </BrowserRouter>
          </LocalizationProvider>
        </ThemeProvider>
      </StyledEngineProvider>
    </Provider>
  </StrictMode>,
);

// ---------------------------------------------------------------------------
// Performance measurement (optional, development only)
// ---------------------------------------------------------------------------

if (import.meta.env.DEV && import.meta.env.VITE_MEASURE_PERF === 'true') {
  import('react-dom/client').then(() => {
    const t = performance.now();
    // eslint-disable-next-line no-console
    console.info(`[Chaos-Sec] App rendered in ${Math.round(t)}ms`);
  });
}
