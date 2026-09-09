import assert from 'node:assert/strict';
import test from 'node:test';

import { validateLiffBuildId } from './verify-liff-build-id.mjs';

test('accepts an explicit LIFF app id that is not bundled inside the SDK', () => {
  assert.doesNotThrow(() => validateLiffBuildId('2010662588-skbKIlSU', 'internal sdk content'));
});

test('rejects a missing or placeholder LIFF app id', () => {
  assert.throws(() => validateLiffBuildId('', ''), /required/);
  assert.throws(() => validateLiffBuildId('replace-after-liff-created', ''), /format/);
});

test('rejects a LIFF id copied from the SDK bundle', () => {
  const sdkSource = 'https://liff.line.me/1656032314-Xgrw5Pmk';
  assert.throws(() => validateLiffBuildId('1656032314-Xgrw5Pmk', sdkSource), /internal SDK/);
});
