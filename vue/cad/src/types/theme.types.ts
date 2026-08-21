export type ThemeSkin = 'elegant' | 'tech' | 'classic'

export type ThemeMode = 'light' | 'dark'

export interface ThemeState {
  skin: ThemeSkin
  mode: ThemeMode
}
