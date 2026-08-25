export type ThemeSkin = 'elegant' | 'tech' | 'classic' | 'starry' | 'bamboo' | 'sage'

export type ThemeMode = 'light' | 'dark'

export interface ThemeState {
  skin: ThemeSkin
  mode: ThemeMode
}
