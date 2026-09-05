export type ThemeSkin = 'elegant' | 'tech' | 'classic' | 'starry' | 'bamboo' | 'sage' | 'juli'

export type ThemeMode = 'light' | 'dark'

export interface ThemeState {
  skin: ThemeSkin
  mode: ThemeMode
}
