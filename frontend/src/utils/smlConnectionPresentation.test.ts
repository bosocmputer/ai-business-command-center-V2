import { describe, expect, it } from 'vitest';
import { formatSMLTestCooldown, resolveSMLJavaWSEndpoint } from './smlConnectionPresentation';

describe('resolveSMLJavaWSEndpoint', () => {
  it('shows the standard JavaWS endpoint once for a base URL', () => {
    expect(resolveSMLJavaWSEndpoint('https://kbgds.iszai.com/')).toBe(
      'https://kbgds.iszai.com/SMLJavaWebService/DotNetFrameWork'
    );
  });

  it('preserves an already-complete JavaWS endpoint without duplicating the path', () => {
    expect(resolveSMLJavaWSEndpoint('https://kbgds.iszai.com/SMLJavaWebService/DotNetFrameWork')).toBe(
      'https://kbgds.iszai.com/SMLJavaWebService/DotNetFrameWork'
    );
  });
});

describe('formatSMLTestCooldown', () => {
  it('uses a clear Thai minute and second countdown', () => {
    expect(formatSMLTestCooldown(61_001)).toBe('1 นาที 2 วินาที');
  });

  it('uses a safe immediate label when no delay remains', () => {
    expect(formatSMLTestCooldown(0)).toBe('สักครู่');
  });
});
