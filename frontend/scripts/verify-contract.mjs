import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';

const schema = await readFile(new URL('../src/api/schema.d.ts', import.meta.url));
const contract = JSON.parse(await readFile(new URL('../api/contract.json', import.meta.url), 'utf8'));
const actualHash = createHash('sha256').update(schema).digest('hex');

if (!/^\d+\.\d+\.\d+$/.test(contract.contractVersion)) throw new Error('contractVersion must use semantic versioning');
if (!/^[0-9a-f]{40}$/.test(contract.backendCommit)) throw new Error('backendCommit must be a full Git SHA');
if (contract.generatedSchemaSha256 !== actualHash) {
  throw new Error('generated API schema differs from api/contract.json; run npm run openapi:generate with the paired backend checkout');
}
