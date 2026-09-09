import { describe, expect, it } from 'vitest';
import { useLayout } from '@/layout/composables/layout';
import {
  APP_THEME_PRESET,
  BLUE_PRIMARY_PALETTE,
  DEFAULT_PRIMARY_COLOR
} from './themeConstants';

type ThemePreset = {
  semantic?: {
    primary?: Record<string, string>;
    colorScheme?: {
      light?: { primary?: { color?: string } };
      dark?: { primary?: { color?: string } };
    };
  };
};

describe('application theme constants', () => {
  it('uses blue as the single default primary color', () => {
    const { layoutConfig } = useLayout();

    expect(DEFAULT_PRIMARY_COLOR).toBe('blue');
    expect(layoutConfig.primary).toBe('blue');
  });

  it('defines every PrimeVue primary shade using the Sakai blue palette', () => {
    expect(Object.keys(BLUE_PRIMARY_PALETTE)).toEqual([
      '50', '100', '200', '300', '400', '500', '600', '700', '800', '900', '950'
    ]);
    expect(BLUE_PRIMARY_PALETTE[500]).toBe('#3b82f6');
    expect(BLUE_PRIMARY_PALETTE[400]).toBe('#60a5fa');
  });

  it('applies blue semantic tokens for light and dark modes without emerald fallbacks', () => {
    const preset = APP_THEME_PRESET as ThemePreset;

    expect(preset.semantic?.primary).toEqual(BLUE_PRIMARY_PALETTE);
    expect(preset.semantic?.colorScheme?.light?.primary?.color).toBe('{primary.500}');
    expect(preset.semantic?.colorScheme?.dark?.primary?.color).toBe('{primary.400}');
    expect(JSON.stringify(preset.semantic?.primary)).not.toContain('emerald');
  });
});
