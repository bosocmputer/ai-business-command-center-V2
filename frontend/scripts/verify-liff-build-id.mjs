import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const liffIdPattern = /^\d{8,}-[A-Za-z0-9_-]{6,}$/;

export function validateLiffBuildId(liffId, sdkSource) {
  if (!liffId) throw new Error('VITE_LINE_LIFF_ID is required.');
  if (!liffIdPattern.test(liffId)) throw new Error('VITE_LINE_LIFF_ID has an invalid format.');
  if (sdkSource.includes(liffId)) {
    throw new Error('VITE_LINE_LIFF_ID matches an internal SDK LIFF ID. Configure the dashboard LIFF app ID explicitly.');
  }
}

async function collectJavaScript(directory) {
  const chunks = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) chunks.push(await collectJavaScript(entryPath));
    else if (/\.(?:c?js|mjs)$/.test(entry.name)) chunks.push(await readFile(entryPath, 'utf8'));
  }
  return chunks.join('\n');
}

async function main() {
  const sdkSource = await collectJavaScript(path.resolve('node_modules/@liff'));
  validateLiffBuildId(process.env.VITE_LINE_LIFF_ID ?? '', sdkSource);
  process.stdout.write('LINE LIFF build configuration verified.\n');
}

const invokedPath = process.argv[1] ? path.resolve(process.argv[1]) : '';
if (invokedPath === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  });
}
