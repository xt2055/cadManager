export type ThemeSkin = 'elegant' | 'tech' | 'classic' | 'starry' | 'bamboo' | 'sage' | 'juli' | 'liquid'

export type ThemeMode = 'light' | 'dark'

export interface ThemeState {
  skin: ThemeSkin
  mode: ThemeMode
}
