import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { readFile, writeFile } from 'node:fs/promises';

const schemaPath = new URL('../src/api/schema.d.ts', import.meta.url);
const contractPath = new URL('../api/contract.json', import.meta.url);
const backendPath = new URL('../../nextstep-dashboard-backend', import.meta.url);

const schema = await readFile(schemaPath);
const existing = JSON.parse(await readFile(contractPath, 'utf8'));
const backendCommit = execFileSync('git', ['-C', backendPath.pathname, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const generatedSchemaSha256 = createHash('sha256').update(schema).digest('hex');

await writeFile(contractPath, `${JSON.stringify({ ...existing, backendCommit, generatedSchemaSha256 }, null, 2)}\n`);
