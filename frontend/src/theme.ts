import { createTheme, type ThemeOptions } from '@mui/material/styles';

const designTokens = {
  colors: {
    primary: '#2563EB',
    primaryLight: '#3B82F6',
    primaryDark: '#1D4ED8',
    secondary: '#7C3AED',
    secondaryLight: '#8B5CF6',
    secondaryDark: '#6D28D9',
    success: '#10B981',
    successLight: '#34D399',
    successDark: '#059669',
    warning: '#F59E0B',
    warningLight: '#FBBF24',
    warningDark: '#D97706',
    error: '#EF4444',
    errorLight: '#F87171',
    errorDark: '#DC2626',
    info: '#06B6D4',
    infoLight: '#22D3EE',
    infoDark: '#0891B2',
  },
  fonts: {
    family: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
  },
  shapes: {
    borderRadius: 8,
    borderRadiusSm: 6,
    borderRadiusMd: 10,
    borderRadiusLg: 14,
    borderRadiusXl: 20,
  },
};

const sharedComponents: ThemeOptions['components'] = {
  MuiCssBaseline: {
    styleOverrides: {
      '*': {
        boxSizing: 'border-box',
      },
      html: {
        WebkitFontSmoothing: 'antialiased',
        MozOsxFontSmoothing: 'grayscale',
      },
      body: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.875rem',
        lineHeight: 1.5,
        color: '#1E293B',
        backgroundColor: '#F8FAFC',
      },
      ':root': {
        '--primary-color': designTokens.colors.primary,
        '--secondary-color': designTokens.colors.secondary,
        '--success-color': designTokens.colors.success,
        '--warning-color': designTokens.colors.warning,
        '--error-color': designTokens.colors.error,
      },
      '::-webkit-scrollbar': {
        width: '6px',
        height: '6px',
      },
      '::-webkit-scrollbar-track': {
        backgroundColor: 'transparent',
      },
      '::-webkit-scrollbar-thumb': {
        backgroundColor: '#CBD5E1',
        borderRadius: '3px',
        '&:hover': {
          backgroundColor: '#94A3B8',
        },
      },
    },
  },
  MuiButton: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 600,
        textTransform: 'none',
        borderRadius: designTokens.shapes.borderRadius,
        padding: '8px 20px',
        fontSize: '0.875rem',
        lineHeight: '1.5',
        boxShadow: 'none',
        transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
        },
        '&:active': {
          boxShadow: 'none',
        },
      },
      contained: {
        '&:hover': {
          boxShadow: '0 4px 12px rgba(37, 99, 235, 0.25)',
        },
      },
      containedSecondary: {
        '&:hover': {
          boxShadow: '0 4px 12px rgba(124, 58, 237, 0.25)',
        },
      },
      outlined: {
        borderWidth: '1.5px',
        '&:hover': {
          borderWidth: '1.5px',
        },
      },
      sizeSmall: {
        padding: '4px 12px',
        fontSize: '0.8125rem',
        borderRadius: designTokens.shapes.borderRadiusSm,
      },
      sizeLarge: {
        padding: '12px 28px',
        fontSize: '1rem',
        borderRadius: designTokens.shapes.borderRadiusMd,
      },
      startIcon: {
        marginRight: 6,
      },
      endIcon: {
        marginLeft: 6,
      },
    },
    defaultProps: {
      disableElevation: true,
      disableRipple: false,
    },
  },
  MuiIconButton: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadius,
        transition: 'all 150ms cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          backgroundColor: 'rgba(37, 99, 235, 0.08)',
        },
      },
    },
  },
  MuiCard: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadiusMd,
        border: '1px solid #E2E8F0',
        boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.06)',
        transition: 'box-shadow 200ms cubic-bezier(0.4, 0, 0.2, 1)',
        overflow: 'hidden',
        '&:hover': {
          boxShadow: '0 4px 12px rgba(0,0,0,0.08)',
        },
      },
    },
    defaultProps: {
      elevation: 0,
    },
  },
  MuiCardHeader: {
    styleOverrides: {
      root: {
        padding: '20px 24px 12px',
      },
      title: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 600,
        fontSize: '1rem',
        lineHeight: 1.5,
        color: '#1E293B',
      },
      subheader: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 400,
        fontSize: '0.875rem',
        lineHeight: 1.5,
        color: '#64748B',
      },
    },
  },
  MuiCardContent: {
    styleOverrides: {
      root: {
        padding: '12px 24px 24px',
        '&:last-child': {
          paddingBottom: 24,
        },
      },
    },
  },
  MuiTable: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
      },
    },
  },
  MuiTableHead: {
    styleOverrides: {
      root: {
        '& .MuiTableCell-root': {
          fontWeight: 600,
          fontSize: '0.75rem',
          textTransform: 'uppercase',
          letterSpacing: '0.05em',
          color: '#475569',
          backgroundColor: '#F8FAFC',
          borderBottom: '2px solid #E2E8F0',
          padding: '12px 16px',
        },
      },
    },
  },
  MuiTableBody: {
    styleOverrides: {
      root: {
        '& .MuiTableCell-root': {
          fontSize: '0.875rem',
          color: '#334155',
          padding: '14px 16px',
          borderBottom: '1px solid #F1F5F9',
        },
        '& .MuiTableRow-root': {
          transition: 'background-color 150ms cubic-bezier(0.4, 0, 0.2, 1)',
          '&:hover': {
            backgroundColor: '#F8FAFC',
          },
          '&.Mui-selected': {
            backgroundColor: 'rgba(37, 99, 235, 0.04)',
            '&:hover': {
              backgroundColor: 'rgba(37, 99, 235, 0.08)',
            },
          },
        },
      },
    },
  },
  MuiTextField: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        '& .MuiOutlinedInput-root': {
          borderRadius: designTokens.shapes.borderRadius,
          fontSize: '0.875rem',
          '& fieldset': {
            borderColor: '#CBD5E1',
          },
          '&:hover fieldset': {
            borderColor: '#94A3B8',
          },
          '&.Mui-focused fieldset': {
            borderColor: designTokens.colors.primary,
            borderWidth: '2px',
          },
          '&.Mui-error fieldset': {
            borderColor: designTokens.colors.error,
          },
        },
        '& .MuiInputLabel-root': {
          fontSize: '0.875rem',
          fontWeight: 500,
          '&.Mui-focused': {
            color: designTokens.colors.primary,
          },
        },
        '& .MuiFormHelperText-root': {
          fontSize: '0.75rem',
          marginTop: 4,
        },
      },
    },
    defaultProps: {
      variant: 'outlined',
      size: 'medium',
    },
  },
  MuiSelect: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadius,
        fontSize: '0.875rem',
      },
    },
    defaultProps: {
      variant: 'outlined',
    },
  },
  MuiChip: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.75rem',
        borderRadius: 20,
        height: 26,
      },
      sizeSmall: {
        height: 22,
        fontSize: '0.6875rem',
      },
      colorSuccess: {
        backgroundColor: 'rgba(16, 185, 129, 0.12)',
        color: '#059669',
        border: '1px solid rgba(16, 185, 129, 0.24)',
      },
      colorWarning: {
        backgroundColor: 'rgba(245, 158, 11, 0.12)',
        color: '#B45309',
        border: '1px solid rgba(245, 158, 11, 0.24)',
      },
      colorError: {
        backgroundColor: 'rgba(239, 68, 68, 0.12)',
        color: '#DC2626',
        border: '1px solid rgba(239, 68, 68, 0.24)',
      },
      colorInfo: {
        backgroundColor: 'rgba(6, 182, 212, 0.12)',
        color: '#0E7490',
        border: '1px solid rgba(6, 182, 212, 0.24)',
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.75rem',
        fontWeight: 500,
        borderRadius: 6,
        padding: '6px 12px',
        backgroundColor: '#1E293B',
        maxWidth: 260,
      },
      arrow: {
        color: '#1E293B',
      },
    },
    defaultProps: {
      arrow: true,
      placement: 'top',
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        borderRadius: designTokens.shapes.borderRadiusLg,
        boxShadow: '0 24px 48px rgba(0,0,0,0.12)',
      },
    },
  },
  MuiDialogTitle: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 600,
        fontSize: '1.125rem',
        color: '#1E293B',
        padding: '24px 24px 12px',
      },
    },
  },
  MuiDialogContent: {
    styleOverrides: {
      root: {
        padding: '12px 24px',
        fontSize: '0.875rem',
        color: '#475569',
      },
    },
  },
  MuiDialogActions: {
    styleOverrides: {
      root: {
        padding: '16px 24px',
        gap: 8,
      },
    },
  },
  MuiAppBar: {
    styleOverrides: {
      root: {
        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      },
    },
  },
  MuiDrawer: {
    styleOverrides: {
      paper: {
        borderRight: '1px solid #E2E8F0',
        boxShadow: 'none',
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadius,
        margin: '2px 8px',
        padding: '8px 12px',
        '&.Mui-selected': {
          backgroundColor: 'rgba(37, 99, 235, 0.08)',
          color: designTokens.colors.primary,
          '&:hover': {
            backgroundColor: 'rgba(37, 99, 235, 0.12)',
          },
          '& .MuiListItemIcon-root': {
            color: designTokens.colors.primary,
          },
        },
      },
    },
  },
  MuiListItemText: {
    styleOverrides: {
      primary: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.875rem',
      },
      secondary: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.75rem',
      },
    },
  },
  MuiListItemIcon: {
    styleOverrides: {
      root: {
        minWidth: 36,
        color: '#64748B',
      },
    },
  },
  MuiAvatar: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 600,
      },
    },
  },
  MuiBadge: {
    styleOverrides: {
      badge: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 600,
        fontSize: '0.6875rem',
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        backgroundImage: 'none',
      },
      elevation0: {
        border: '1px solid #E2E8F0',
      },
      elevation1: {
        boxShadow: '0 1px 3px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.06)',
      },
      elevation2: {
        boxShadow: '0 4px 6px -1px rgba(0,0,0,0.06), 0 2px 4px -2px rgba(0,0,0,0.06)',
      },
    },
    defaultProps: {
      elevation: 0,
    },
  },
  MuiTab: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.875rem',
        textTransform: 'none',
        minHeight: 44,
      },
    },
  },
  MuiStepper: {
    styleOverrides: {
      root: {
        padding: '8px 0',
      },
    },
  },
  MuiStepLabel: {
    styleOverrides: {
      label: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.875rem',
        '&.Mui-active': {
          fontWeight: 600,
        },
        '&.Mui-completed': {
          fontWeight: 600,
        },
      },
    },
  },
  MuiAlert: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadius,
        fontFamily: designTokens.fonts.family,
        fontSize: '0.875rem',
      },
      standardSuccess: {
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        color: '#065F46',
        border: '1px solid rgba(16, 185, 129, 0.2)',
      },
      standardWarning: {
        backgroundColor: 'rgba(245, 158, 11, 0.08)',
        color: '#92400E',
        border: '1px solid rgba(245, 158, 11, 0.2)',
      },
      standardError: {
        backgroundColor: 'rgba(239, 68, 68, 0.08)',
        color: '#991B1B',
        border: '1px solid rgba(239, 68, 68, 0.2)',
      },
      standardInfo: {
        backgroundColor: 'rgba(6, 182, 212, 0.08)',
        color: '#155E75',
        border: '1px solid rgba(6, 182, 212, 0.2)',
      },
    },
  },
  MuiLinearProgress: {
    styleOverrides: {
      root: {
        borderRadius: 4,
        height: 6,
        backgroundColor: '#E2E8F0',
      },
      bar: {
        borderRadius: 4,
      },
    },
  },
  MuiCircularProgress: {
    defaultProps: {
      size: 20,
      thickness: 5,
    },
  },
  MuiSwitch: {
    styleOverrides: {
      root: {
        padding: 8,
      },
      switchBase: {
        '&.Mui-checked + .MuiSwitch-track': {
          opacity: 0.12,
        },
      },
      track: {
        opacity: 0.2,
        borderRadius: 14,
      },
      thumb: {
        boxShadow: '0 2px 4px rgba(0,0,0,0.15)',
      },
    },
  },
  MuiBreadcrumbs: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.875rem',
      },
      li: {
        '& .MuiLink-root': {
          fontWeight: 500,
          color: '#64748B',
          '&:hover': {
            color: designTokens.colors.primary,
          },
        },
        '&.MuiBreadcrumbs-separator': {
          color: '#CBD5E1',
        },
      },
    },
  },
  MuiLink: {
    defaultProps: {
      underline: 'hover',
    },
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.875rem',
      },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        borderRadius: designTokens.shapes.borderRadius,
        boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
        border: '1px solid #E2E8F0',
      },
      list: {
        padding: '4px 0',
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.875rem',
        fontWeight: 500,
        padding: '8px 16px',
        borderRadius: 4,
        margin: '0 4px',
        minWidth: 'calc(100% - 8px)',
      },
    },
  },
  MuiPagination: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
      },
      ul: {
        gap: 4,
      },
    },
  },
  MuiPaginationItem: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        fontWeight: 500,
        fontSize: '0.875rem',
        borderRadius: designTokens.shapes.borderRadius,
        '&.Mui-selected': {
          fontWeight: 600,
        },
      },
    },
  },
  MuiSkeleton: {
    styleOverrides: {
      root: {
        borderRadius: designTokens.shapes.borderRadiusSm,
      },
    },
  },
  MuiSnackbar: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
      },
    },
  },
  MuiAutocomplete: {
    styleOverrides: {
      root: {
        fontFamily: designTokens.fonts.family,
        '& .MuiOutlinedInput-root': {
          fontSize: '0.875rem',
          borderRadius: designTokens.shapes.borderRadius,
        },
        '& .MuiAutocomplete-tag': {
          fontSize: '0.75rem',
        },
      },
      paper: {
        borderRadius: designTokens.shapes.borderRadius,
        boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
        border: '1px solid #E2E8F0',
      },
      option: {
        fontFamily: designTokens.fonts.family,
        fontSize: '0.875rem',
      },
    },
  },
};

const palette = {
  primary: {
    main: designTokens.colors.primary,
    light: designTokens.colors.primaryLight,
    dark: designTokens.colors.primaryDark,
    contrastText: '#FFFFFF',
  },
  secondary: {
    main: designTokens.colors.secondary,
    light: designTokens.colors.secondaryLight,
    dark: designTokens.colors.secondaryDark,
    contrastText: '#FFFFFF',
  },
  success: {
    main: designTokens.colors.success,
    light: designTokens.colors.successLight,
    dark: designTokens.colors.successDark,
    contrastText: '#FFFFFF',
  },
  warning: {
    main: designTokens.colors.warning,
    light: designTokens.colors.warningLight,
    dark: designTokens.colors.warningDark,
    contrastText: '#FFFFFF',
  },
  error: {
    main: designTokens.colors.error,
    light: designTokens.colors.errorLight,
    dark: designTokens.colors.errorDark,
    contrastText: '#FFFFFF',
  },
  info: {
    main: designTokens.colors.info,
    light: designTokens.colors.infoLight,
    dark: designTokens.colors.infoDark,
    contrastText: '#FFFFFF',
  },
  background: {
    default: '#F8FAFC',
    paper: '#FFFFFF',
  },
  text: {
    primary: '#1E293B',
    secondary: '#64748B',
    disabled: '#94A3B8',
  },
  divider: '#E2E8F0',
  action: {
    active: '#475569',
    hover: 'rgba(37, 99, 235, 0.04)',
    hoverOpacity: 0.04,
    selected: 'rgba(37, 99, 235, 0.08)',
    selectedOpacity: 0.08,
    disabled: '#94A3B8',
    disabledBackground: '#F1F5F9',
    disabledOpacity: 0.38,
    focus: 'rgba(37, 99, 235, 0.12)',
    focusOpacity: 0.12,
    activatedOpacity: 0.12,
  },
  grey: {
    50: '#F8FAFC',
    100: '#F1F5F9',
    200: '#E2E8F0',
    300: '#CBD5E1',
    400: '#94A3B8',
    500: '#64748B',
    600: '#475569',
    700: '#334155',
    800: '#1E293B',
    900: '#0F172A',
    A100: '#F1F5F9',
    A200: '#E2E8F0',
    A400: '#94A3B8',
    A700: '#475569',
  },
};

const lightTheme = createTheme({
  palette: {
    mode: 'light',
    ...palette,
  },
  typography: {
    fontFamily: designTokens.fonts.family,
    htmlFontSize: 16,
    fontSize: 14,
    h1: {
      fontSize: '2.5rem',
      fontWeight: 800,
      lineHeight: 1.2,
      letterSpacing: '-0.02em',
      color: '#0F172A',
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 700,
      lineHeight: 1.3,
      letterSpacing: '-0.01em',
      color: '#1E293B',
    },
    h3: {
      fontSize: '1.5rem',
      fontWeight: 700,
      lineHeight: 1.4,
      color: '#1E293B',
    },
    h4: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.4,
      color: '#1E293B',
    },
    h5: {
      fontSize: '1rem',
      fontWeight: 600,
      lineHeight: 1.5,
      color: '#1E293B',
    },
    h6: {
      fontSize: '0.875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      color: '#1E293B',
    },
    subtitle1: {
      fontSize: '1rem',
      fontWeight: 500,
      lineHeight: 1.5,
      color: '#475569',
    },
    subtitle2: {
      fontSize: '0.875rem',
      fontWeight: 500,
      lineHeight: 1.5,
      color: '#64748B',
    },
    body1: {
      fontSize: '1rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#334155',
    },
    body2: {
      fontSize: '0.875rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#334155',
    },
    button: {
      fontSize: '0.875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      textTransform: 'none',
    },
    caption: {
      fontSize: '0.75rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#64748B',
    },
    overline: {
      fontSize: '0.6875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      textTransform: 'uppercase',
      letterSpacing: '0.08em',
      color: '#64748B',
    },
  },
  shape: {
    borderRadius: designTokens.shapes.borderRadius,
  },
  shadows: [
    'none',
    '0 1px 2px rgba(0,0,0,0.04)',
    '0 1px 3px rgba(0,0,0,0.04), 0 1px 2px rgba(0,0,0,0.06)',
    '0 4px 6px -1px rgba(0,0,0,0.04), 0 2px 4px -2px rgba(0,0,0,0.06)',
    '0 4px 6px -1px rgba(0,0,0,0.06), 0 2px 4px -2px rgba(0,0,0,0.06)',
    '0 8px 12px -2px rgba(0,0,0,0.06), 0 4px 6px -2px rgba(0,0,0,0.06)',
    '0 10px 20px -2px rgba(0,0,0,0.08)',
    '0 16px 32px -4px rgba(0,0,0,0.1)',
    '0 20px 40px -8px rgba(0,0,0,0.12)',
    '0 24px 48px -12px rgba(0,0,0,0.14)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
    '0 32px 64px -16px rgba(0,0,0,0.16)',
  ],
  spacing: (factor: number) => `${0.25 * factor}rem`,
  components: sharedComponents,
});

const darkPalette = {
  primary: {
    main: '#3B82F6',
    light: '#60A5FA',
    dark: '#2563EB',
    contrastText: '#FFFFFF',
  },
  secondary: {
    main: '#8B5CF6',
    light: '#A78BFA',
    dark: '#7C3AED',
    contrastText: '#FFFFFF',
  },
  success: {
    main: '#34D399',
    light: '#6EE7B7',
    dark: '#10B981',
    contrastText: '#0F172A',
  },
  warning: {
    main: '#FBBF24',
    light: '#FDE68A',
    dark: '#F59E0B',
    contrastText: '#0F172A',
  },
  error: {
    main: '#F87171',
    light: '#FCA5A5',
    dark: '#EF4444',
    contrastText: '#FFFFFF',
  },
  info: {
    main: '#22D3EE',
    light: '#67E8F9',
    dark: '#06B6D4',
    contrastText: '#0F172A',
  },
  background: {
    default: '#0F172A',
    paper: '#1E293B',
  },
  text: {
    primary: '#F1F5F9',
    secondary: '#94A3B8',
    disabled: '#64748B',
  },
  divider: '#334155',
  action: {
    active: '#CBD5E1',
    hover: 'rgba(59, 130, 246, 0.08)',
    hoverOpacity: 0.08,
    selected: 'rgba(59, 130, 246, 0.16)',
    selectedOpacity: 0.16,
    disabled: '#64748B',
    disabledBackground: '#334155',
    disabledOpacity: 0.38,
    focus: 'rgba(59, 130, 246, 0.24)',
    focusOpacity: 0.24,
    activatedOpacity: 0.16,
  },
  grey: {
    50: '#F8FAFC',
    100: '#F1F5F9',
    200: '#E2E8F0',
    300: '#CBD5E1',
    400: '#94A3B8',
    500: '#64748B',
    600: '#475569',
    700: '#334155',
    800: '#1E293B',
    900: '#0F172A',
    A100: '#1E293B',
    A200: '#334155',
    A400: '#64748B',
    A700: '#CBD5E1',
  },
};

const darkTheme = createTheme({
  palette: {
    mode: 'dark',
    ...darkPalette,
  },
  typography: {
    fontFamily: designTokens.fonts.family,
    htmlFontSize: 16,
    fontSize: 14,
    h1: {
      fontSize: '2.5rem',
      fontWeight: 800,
      lineHeight: 1.2,
      letterSpacing: '-0.02em',
      color: '#F8FAFC',
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 700,
      lineHeight: 1.3,
      letterSpacing: '-0.01em',
      color: '#F1F5F9',
    },
    h3: {
      fontSize: '1.5rem',
      fontWeight: 700,
      lineHeight: 1.4,
      color: '#F1F5F9',
    },
    h4: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.4,
      color: '#F1F5F9',
    },
    h5: {
      fontSize: '1rem',
      fontWeight: 600,
      lineHeight: 1.5,
      color: '#F1F5F9',
    },
    h6: {
      fontSize: '0.875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      color: '#F1F5F9',
    },
    subtitle1: {
      fontSize: '1rem',
      fontWeight: 500,
      lineHeight: 1.5,
      color: '#CBD5E1',
    },
    subtitle2: {
      fontSize: '0.875rem',
      fontWeight: 500,
      lineHeight: 1.5,
      color: '#94A3B8',
    },
    body1: {
      fontSize: '1rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#CBD5E1',
    },
    body2: {
      fontSize: '0.875rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#CBD5E1',
    },
    button: {
      fontSize: '0.875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      textTransform: 'none',
    },
    caption: {
      fontSize: '0.75rem',
      fontWeight: 400,
      lineHeight: 1.5,
      color: '#94A3B8',
    },
    overline: {
      fontSize: '0.6875rem',
      fontWeight: 600,
      lineHeight: 1.5,
      textTransform: 'uppercase',
      letterSpacing: '0.08em',
      color: '#94A3B8',
    },
  },
  shape: {
    borderRadius: designTokens.shapes.borderRadius,
  },
  shadows: Array(25).fill('none') as unknown as import('@mui/material/styles').Shadows,
  spacing: (factor: number) => `${0.25 * factor}rem`,
  components: {
    ...sharedComponents,
    MuiCard: {
      ...sharedComponents.MuiCard,
      styleOverrides: {
        root: {
          borderRadius: designTokens.shapes.borderRadiusMd,
          border: '1px solid #334155',
          boxShadow: 'none',
          backgroundColor: '#1E293B',
          transition: 'box-shadow 200ms cubic-bezier(0.4, 0, 0.2, 1)',
          overflow: 'hidden',
          '&:hover': {
            boxShadow: '0 4px 12px rgba(0,0,0,0.25)',
          },
        },
      },
    },
    MuiPaper: {
      ...sharedComponents.MuiPaper,
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: '#1E293B',
        },
        elevation0: {
          border: '1px solid #334155',
        },
      },
    },
    MuiTableHead: {
      styleOverrides: {
        root: {
          '& .MuiTableCell-root': {
            fontWeight: 600,
            fontSize: '0.75rem',
            textTransform: 'uppercase',
            letterSpacing: '0.05em',
            color: '#94A3B8',
            backgroundColor: '#1E293B',
            borderBottom: '2px solid #334155',
            padding: '12px 16px',
          },
        },
      },
    },
    MuiTableBody: {
      styleOverrides: {
        root: {
          '& .MuiTableCell-root': {
            fontSize: '0.875rem',
            color: '#CBD5E1',
            padding: '14px 16px',
            borderBottom: '1px solid #334155',
          },
          '& .MuiTableRow-root': {
            transition: 'background-color 150ms cubic-bezier(0.4, 0, 0.2, 1)',
            '&:hover': {
              backgroundColor: 'rgba(59, 130, 246, 0.04)',
            },
          },
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            borderRadius: designTokens.shapes.borderRadius,
            fontSize: '0.875rem',
            '& fieldset': {
              borderColor: '#475569',
            },
            '&:hover fieldset': {
              borderColor: '#64748B',
            },
            '&.Mui-focused fieldset': {
              borderColor: darkPalette.primary.main,
              borderWidth: '2px',
            },
          },
          '& .MuiInputLabel-root': {
            color: '#94A3B8',
            '&.Mui-focused': {
              color: darkPalette.primary.main,
            },
          },
        },
      },
      defaultProps: {
        variant: 'outlined',
        size: 'medium',
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          borderRight: '1px solid #334155',
          backgroundColor: '#1E293B',
          boxShadow: 'none',
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          boxShadow: '0 1px 3px rgba(0,0,0,0.3)',
          backgroundColor: '#1E293B',
          borderBottom: '1px solid #334155',
        },
      },
    },
    MuiChip: {
      ...sharedComponents.MuiChip,
      styleOverrides: {
        ...sharedComponents.MuiChip?.styleOverrides,
        colorSuccess: {
          backgroundColor: 'rgba(52, 211, 153, 0.15)',
          color: '#6EE7B7',
          border: '1px solid rgba(52, 211, 153, 0.3)',
        },
        colorWarning: {
          backgroundColor: 'rgba(251, 191, 36, 0.15)',
          color: '#FDE68A',
          border: '1px solid rgba(251, 191, 36, 0.3)',
        },
        colorError: {
          backgroundColor: 'rgba(248, 113, 113, 0.15)',
          color: '#FCA5A5',
          border: '1px solid rgba(248, 113, 113, 0.3)',
        },
        colorInfo: {
          backgroundColor: 'rgba(34, 211, 238, 0.15)',
          color: '#67E8F9',
          border: '1px solid rgba(34, 211, 238, 0.3)',
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: designTokens.shapes.borderRadius,
          fontFamily: designTokens.fonts.family,
          fontSize: '0.875rem',
        },
        standardSuccess: {
          backgroundColor: 'rgba(52, 211, 153, 0.1)',
          color: '#6EE7B7',
          border: '1px solid rgba(52, 211, 153, 0.25)',
        },
        standardWarning: {
          backgroundColor: 'rgba(251, 191, 36, 0.1)',
          color: '#FDE68A',
          border: '1px solid rgba(251, 191, 36, 0.25)',
        },
        standardError: {
          backgroundColor: 'rgba(248, 113, 113, 0.1)',
          color: '#FCA5A5',
          border: '1px solid rgba(248, 113, 113, 0.25)',
        },
        standardInfo: {
          backgroundColor: 'rgba(34, 211, 238, 0.1)',
          color: '#67E8F9',
          border: '1px solid rgba(34, 211, 238, 0.25)',
        },
      },
    },
  },
});

export { lightTheme, darkTheme, designTokens };
export default lightTheme;
