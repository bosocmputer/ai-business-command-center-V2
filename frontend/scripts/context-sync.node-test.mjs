import assert from 'node:assert/strict';
import test from 'node:test';
import ts from 'typescript';

import { extractRouteRows, renderRouteInventory, replaceGeneratedBlock } from './context-sync.mjs';

test('extractRouteRows resolves nested route paths and metadata', () => {
  const source = `
    const router = createRouter({
      routes: [
        { path: '/', redirect: '/app' },
        {
          path: '/admin',
          children: [
            { path: '', name: 'admin-home', meta: { pageTitle: 'ภาพรวม' } },
            { path: 'tenants/:tenantId', name: 'admin-tenant' }
          ]
        },
        { path: '/app', name: 'viewer-home' }
      ]
    });
  `;
  assert.deepEqual(extractRouteRows(source, ts), [
    { path: '/', name: '—', surface: 'Public', pageTitle: '—' },
    { path: '/admin', name: '—', surface: 'Admin', pageTitle: '—' },
    { path: '/admin', name: 'admin-home', surface: 'Admin', pageTitle: 'ภาพรวม' },
    { path: '/admin/tenants/:tenantId', name: 'admin-tenant', surface: 'Admin', pageTitle: '—' },
    { path: '/app', name: 'viewer-home', surface: 'Viewer', pageTitle: '—' }
  ]);
});

test('renderRouteInventory escapes table delimiters', () => {
  const rendered = renderRouteInventory([
    { path: '/app/a|b', name: 'route', surface: 'Viewer', pageTitle: 'A|B' }
  ]);
  assert.match(rendered, /\/app\/a\\\|b/);
  assert.match(rendered, /A\\\|B/);
});

test('replaceGeneratedBlock changes only marker contents', () => {
  const original = `before\n<!-- BEGIN GENERATED: ROUTE_INVENTORY -->\nold\n<!-- END GENERATED: ROUTE_INVENTORY -->\nafter\n`;
  const updated = replaceGeneratedBlock(original, 'new');
  assert.equal(
    updated,
    `before\n<!-- BEGIN GENERATED: ROUTE_INVENTORY -->\nnew\n<!-- END GENERATED: ROUTE_INVENTORY -->\nafter\n`
  );
  assert.throws(() => replaceGeneratedBlock('no markers', 'new'), /marker/);
});
