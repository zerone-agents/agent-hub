export type ThemeAppearance = 'light' | 'dark'
export type ThemePreference = ThemeAppearance | 'system'

export interface ThemeColors {
  background: string
  foreground: string
  card: string
  cardForeground: string
  popover: string
  popoverForeground: string
  primary: string
  primaryForeground: string
  secondary: string
  secondaryForeground: string
  muted: string
  mutedForeground: string
  accent: string
  accentForeground: string
  destructive: string
  border: string
  input: string
  ring: string
  chart1: string
  chart2: string
  chart3: string
  chart4: string
  chart5: string
  sidebar: string
  sidebarForeground: string
  sidebarPrimary: string
  sidebarPrimaryForeground: string
  sidebarAccent: string
  sidebarAccentForeground: string
  sidebarBorder: string
  sidebarRing: string
}

export interface AppTheme {
  id: string
  label: string
  order: number
  description: string
  codeTheme: {
    light: string
    dark: string
  }
  shikiTheme: {
    light: string
    dark: string
  }
  light: ThemeColors
  dark: ThemeColors
}

export const themes: AppTheme[] = [
  {
    id: 'terracotta',
    label: 'Terracotta',
    order: 1,
    description:
      'Warm porcelain surfaces with charcoal text and terracotta accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#f3f1ed',
      foreground: '#211f1d',
      card: '#fbfaf8',
      cardForeground: '#211f1d',
      popover: '#fffdfa',
      popoverForeground: '#211f1d',
      primary: '#d96f4f',
      primaryForeground: '#ffffff',
      secondary: '#ebe7e1',
      secondaryForeground: '#38332f',
      muted: '#eeebe6',
      mutedForeground: '#6f6963',
      accent: '#f0dfd7',
      accentForeground: '#71331f',
      destructive: '#c94b45',
      border: '#dcd7d0',
      input: '#d4cec6',
      ring: '#d96f4f',
      chart1: '#d96f4f',
      chart2: '#57756a',
      chart3: '#b88b4a',
      chart4: '#6e7f93',
      chart5: '#a85f68',
      sidebar: '#eae7e2',
      sidebarForeground: '#282522',
      sidebarPrimary: '#d96f4f',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#dfdad3',
      sidebarAccentForeground: '#4b332a',
      sidebarBorder: '#d2ccc4',
      sidebarRing: '#d96f4f'
    },
    dark: {
      background: '#1b1917',
      foreground: '#f2eee9',
      card: '#24211e',
      cardForeground: '#f2eee9',
      popover: '#292521',
      popoverForeground: '#f2eee9',
      primary: '#e68a6b',
      primaryForeground: '#28140d',
      secondary: '#302c28',
      secondaryForeground: '#f2eee9',
      muted: '#302c28',
      mutedForeground: '#b9b1aa',
      accent: '#49342b',
      accentForeground: '#ffd8c8',
      destructive: '#ef837c',
      border: '#49423c',
      input: '#514943',
      ring: '#e68a6b',
      chart1: '#e68a6b',
      chart2: '#83a89a',
      chart3: '#d4a867',
      chart4: '#93a5ba',
      chart5: '#ce8991',
      sidebar: '#211e1b',
      sidebarForeground: '#f2eee9',
      sidebarPrimary: '#e68a6b',
      sidebarPrimaryForeground: '#28140d',
      sidebarAccent: '#3a332e',
      sidebarAccentForeground: '#f2eee9',
      sidebarBorder: '#403934',
      sidebarRing: '#e68a6b'
    }
  },
  {
    id: 'slack',
    label: 'Slack',
    order: 2,
    description: 'Warm neutrals with Slack-inspired aubergine and teal accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#ffffff',
      foreground: '#1d1c1d',
      card: '#ffffff',
      cardForeground: '#1d1c1d',
      popover: '#ffffff',
      popoverForeground: '#1d1c1d',
      primary: '#611f69',
      primaryForeground: '#ffffff',
      secondary: '#f8f5f8',
      secondaryForeground: '#3f0e40',
      muted: '#f3f1f3',
      mutedForeground: '#696069',
      accent: '#eee8ee',
      accentForeground: '#3f0e40',
      destructive: '#d72c3d',
      border: '#ddd7dd',
      input: '#ddd7dd',
      ring: '#611f69',
      chart1: '#611f69',
      chart2: '#007a5a',
      chart3: '#e8912d',
      chart4: '#36c5f0',
      chart5: '#e01e5a',
      sidebar: '#f7f4f7',
      sidebarForeground: '#1d1c1d',
      sidebarPrimary: '#611f69',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#e9e2e9',
      sidebarAccentForeground: '#3f0e40',
      sidebarBorder: '#ddd7dd',
      sidebarRing: '#611f69'
    },
    dark: {
      background: '#1a171a',
      foreground: '#f9f7f9',
      card: '#241f24',
      cardForeground: '#f9f7f9',
      popover: '#241f24',
      popoverForeground: '#f9f7f9',
      primary: '#d1a7e0',
      primaryForeground: '#28122c',
      secondary: '#302a30',
      secondaryForeground: '#f9f7f9',
      muted: '#302a30',
      mutedForeground: '#b9b1b9',
      accent: '#403940',
      accentForeground: '#f9f7f9',
      destructive: '#ff8a98',
      border: '#514951',
      input: '#514951',
      ring: '#d1a7e0',
      chart1: '#d1a7e0',
      chart2: '#2eb67d',
      chart3: '#ecb22e',
      chart4: '#36c5f0',
      chart5: '#e01e5a',
      sidebar: '#221d22',
      sidebarForeground: '#f9f7f9',
      sidebarPrimary: '#d1a7e0',
      sidebarPrimaryForeground: '#28122c',
      sidebarAccent: '#403940',
      sidebarAccentForeground: '#f9f7f9',
      sidebarBorder: '#514951',
      sidebarRing: '#d1a7e0'
    }
  },
  {
    id: 'discord',
    label: 'Discord',
    order: 3,
    description: 'Cool neutral surfaces with Discord blurple accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#f2f3f5',
      foreground: '#232428',
      card: '#ffffff',
      cardForeground: '#232428',
      popover: '#ffffff',
      popoverForeground: '#232428',
      primary: '#5865f2',
      primaryForeground: '#ffffff',
      secondary: '#e9eaec',
      secondaryForeground: '#313338',
      muted: '#e3e5e8',
      mutedForeground: '#5c5e66',
      accent: '#e0e3ff',
      accentForeground: '#3139a7',
      destructive: '#da373c',
      border: '#d4d7dc',
      input: '#c9ccd2',
      ring: '#5865f2',
      chart1: '#5865f2',
      chart2: '#248046',
      chart3: '#f0b232',
      chart4: '#00a8fc',
      chart5: '#eb459e',
      sidebar: '#e3e5e8',
      sidebarForeground: '#2b2d31',
      sidebarPrimary: '#5865f2',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#d5d8ff',
      sidebarAccentForeground: '#3139a7',
      sidebarBorder: '#cdd0d5',
      sidebarRing: '#5865f2'
    },
    dark: {
      background: '#313338',
      foreground: '#f2f3f5',
      card: '#2b2d31',
      cardForeground: '#f2f3f5',
      popover: '#232428',
      popoverForeground: '#f2f3f5',
      primary: '#7983f5',
      primaryForeground: '#111214',
      secondary: '#383a40',
      secondaryForeground: '#f2f3f5',
      muted: '#3f4147',
      mutedForeground: '#b5bac1',
      accent: '#404675',
      accentForeground: '#e0e3ff',
      destructive: '#f27a7e',
      border: '#4e5058',
      input: '#5c5e66',
      ring: '#7983f5',
      chart1: '#7983f5',
      chart2: '#23a559',
      chart3: '#f0b232',
      chart4: '#00a8fc',
      chart5: '#f47bba',
      sidebar: '#1e1f22',
      sidebarForeground: '#dbdee1',
      sidebarPrimary: '#7983f5',
      sidebarPrimaryForeground: '#111214',
      sidebarAccent: '#35375c',
      sidebarAccentForeground: '#f2f3f5',
      sidebarBorder: '#3f4147',
      sidebarRing: '#7983f5'
    }
  },
  {
    id: 'twitter',
    label: 'Twitter',
    order: 4,
    description: 'Crisp monochrome surfaces with Twitter blue accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#ffffff',
      foreground: '#0f1419',
      card: '#ffffff',
      cardForeground: '#0f1419',
      popover: '#ffffff',
      popoverForeground: '#0f1419',
      primary: '#1d9bf0',
      primaryForeground: '#ffffff',
      secondary: '#eff3f4',
      secondaryForeground: '#0f1419',
      muted: '#f7f9f9',
      mutedForeground: '#536471',
      accent: '#e8f5fd',
      accentForeground: '#0c6da3',
      destructive: '#f4212e',
      border: '#cfd9de',
      input: '#b9cad3',
      ring: '#1d9bf0',
      chart1: '#1d9bf0',
      chart2: '#00ba7c',
      chart3: '#ffd400',
      chart4: '#7856ff',
      chart5: '#f91880',
      sidebar: '#f7f9f9',
      sidebarForeground: '#0f1419',
      sidebarPrimary: '#1d9bf0',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#e8f5fd',
      sidebarAccentForeground: '#0c6da3',
      sidebarBorder: '#cfd9de',
      sidebarRing: '#1d9bf0'
    },
    dark: {
      background: '#0f1419',
      foreground: '#e7e9ea',
      card: '#16181c',
      cardForeground: '#e7e9ea',
      popover: '#1d1f23',
      popoverForeground: '#e7e9ea',
      primary: '#1d9bf0',
      primaryForeground: '#ffffff',
      secondary: '#202327',
      secondaryForeground: '#e7e9ea',
      muted: '#202327',
      mutedForeground: '#8b98a5',
      accent: '#16394f',
      accentForeground: '#8ecdf7',
      destructive: '#ff6670',
      border: '#38444d',
      input: '#536471',
      ring: '#1d9bf0',
      chart1: '#1d9bf0',
      chart2: '#00ba7c',
      chart3: '#ffd400',
      chart4: '#7856ff',
      chart5: '#f91880',
      sidebar: '#14171a',
      sidebarForeground: '#e7e9ea',
      sidebarPrimary: '#1d9bf0',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#16394f',
      sidebarAccentForeground: '#e7e9ea',
      sidebarBorder: '#38444d',
      sidebarRing: '#1d9bf0'
    }
  },
  {
    id: 'github',
    label: 'GitHub',
    order: 5,
    description: 'Developer-focused gray surfaces with GitHub blue accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#f6f8fa',
      foreground: '#1f2328',
      card: '#ffffff',
      cardForeground: '#1f2328',
      popover: '#ffffff',
      popoverForeground: '#1f2328',
      primary: '#0969da',
      primaryForeground: '#ffffff',
      secondary: '#f6f8fa',
      secondaryForeground: '#1f2328',
      muted: '#eaeef2',
      mutedForeground: '#59636e',
      accent: '#ddf4ff',
      accentForeground: '#0550ae',
      destructive: '#cf222e',
      border: '#d0d7de',
      input: '#afb8c1',
      ring: '#0969da',
      chart1: '#0969da',
      chart2: '#1a7f37',
      chart3: '#9a6700',
      chart4: '#8250df',
      chart5: '#bf3989',
      sidebar: '#f0f3f6',
      sidebarForeground: '#1f2328',
      sidebarPrimary: '#0969da',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#ddf4ff',
      sidebarAccentForeground: '#0550ae',
      sidebarBorder: '#d0d7de',
      sidebarRing: '#0969da'
    },
    dark: {
      background: '#0d1117',
      foreground: '#e6edf3',
      card: '#161b22',
      cardForeground: '#e6edf3',
      popover: '#1c2128',
      popoverForeground: '#e6edf3',
      primary: '#58a6ff',
      primaryForeground: '#0d1117',
      secondary: '#21262d',
      secondaryForeground: '#e6edf3',
      muted: '#21262d',
      mutedForeground: '#8b949e',
      accent: '#1f3b5d',
      accentForeground: '#cae8ff',
      destructive: '#f85149',
      border: '#30363d',
      input: '#484f58',
      ring: '#58a6ff',
      chart1: '#58a6ff',
      chart2: '#3fb950',
      chart3: '#d29922',
      chart4: '#a371f7',
      chart5: '#db61a2',
      sidebar: '#010409',
      sidebarForeground: '#e6edf3',
      sidebarPrimary: '#58a6ff',
      sidebarPrimaryForeground: '#0d1117',
      sidebarAccent: '#1f3b5d',
      sidebarAccentForeground: '#e6edf3',
      sidebarBorder: '#30363d',
      sidebarRing: '#58a6ff'
    }
  },
  {
    id: 'facebook',
    label: 'Facebook',
    order: 6,
    description: 'Soft cool grays with Facebook blue accents',
    codeTheme: {
      light: 'ghcolors',
      dark: 'oneDark'
    },
    shikiTheme: {
      light: 'github-light',
      dark: 'github-dark'
    },
    light: {
      background: '#f0f2f5',
      foreground: '#050505',
      card: '#ffffff',
      cardForeground: '#050505',
      popover: '#ffffff',
      popoverForeground: '#050505',
      primary: '#0866ff',
      primaryForeground: '#ffffff',
      secondary: '#e4e6eb',
      secondaryForeground: '#050505',
      muted: '#e4e6eb',
      mutedForeground: '#65676b',
      accent: '#e7f3ff',
      accentForeground: '#0754c9',
      destructive: '#e41e3f',
      border: '#ced0d4',
      input: '#bcc0c4',
      ring: '#0866ff',
      chart1: '#0866ff',
      chart2: '#31a24c',
      chart3: '#f7b928',
      chart4: '#a033ff',
      chart5: '#f5533d',
      sidebar: '#e9ebee',
      sidebarForeground: '#050505',
      sidebarPrimary: '#0866ff',
      sidebarPrimaryForeground: '#ffffff',
      sidebarAccent: '#dceaff',
      sidebarAccentForeground: '#0754c9',
      sidebarBorder: '#ced0d4',
      sidebarRing: '#0866ff'
    },
    dark: {
      background: '#18191a',
      foreground: '#e4e6eb',
      card: '#242526',
      cardForeground: '#e4e6eb',
      popover: '#2d2e2f',
      popoverForeground: '#e4e6eb',
      primary: '#4599ff',
      primaryForeground: '#101820',
      secondary: '#3a3b3c',
      secondaryForeground: '#e4e6eb',
      muted: '#3a3b3c',
      mutedForeground: '#b0b3b8',
      accent: '#263c5c',
      accentForeground: '#d8e9ff',
      destructive: '#ff667e',
      border: '#3e4042',
      input: '#5c5e62',
      ring: '#4599ff',
      chart1: '#4599ff',
      chart2: '#45bd62',
      chart3: '#f7b928',
      chart4: '#b768ff',
      chart5: '#ff796b',
      sidebar: '#202122',
      sidebarForeground: '#e4e6eb',
      sidebarPrimary: '#4599ff',
      sidebarPrimaryForeground: '#101820',
      sidebarAccent: '#263c5c',
      sidebarAccentForeground: '#e4e6eb',
      sidebarBorder: '#3e4042',
      sidebarRing: '#4599ff'
    }
  }
]

export const defaultThemeId = 'terracotta'

export function getTheme(themeId: string): AppTheme {
  return themes.find((theme) => theme.id === themeId) ?? themes[0]
}
