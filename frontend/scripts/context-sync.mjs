#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const START_MARKER = '<!-- BEGIN GENERATED: ROUTE_INVENTORY -->';
const END_MARKER = '<!-- END GENERATED: ROUTE_INVENTORY -->';

function property(object, name) {
  return object.properties.find((item) => {
    if (!ts.isPropertyAssignment(item)) return false;
    if (ts.isIdentifier(item.name) || ts.isStringLiteral(item.name)) return item.name.text === name;
    return false;
  });
}

function literalText(node) {
  if (!node) return undefined;
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  return undefined;
}

function joinRoutePath(parent, child) {
  if (child.startsWith('/')) return child;
  if (!parent) return child ? `/${child}` : '/';
  if (!child) return parent;
  return `${parent.replace(/\/$/, '')}/${child}`;
}

function surfaceFor(routePath) {
  if (routePath === '/admin' || routePath.startsWith('/admin/')) return 'Admin';
  if (routePath === '/app' || routePath.startsWith('/app/')) return 'Viewer';
  return 'Public';
}

export function extractRouteRows(sourceText, typescript = ts) {
  const source = typescript.createSourceFile('router.ts', sourceText, typescript.ScriptTarget.Latest, true);
  let routes;
  const visit = (node) => {
    if (
      typescript.isCallExpression(node) &&
      typescript.isIdentifier(node.expression) &&
      node.expression.text === 'createRouter' &&
      node.arguments.length > 0 &&
      typescript.isObjectLiteralExpression(node.arguments[0])
    ) {
      const routesProperty = property(node.arguments[0], 'routes');
      if (routesProperty && typescript.isArrayLiteralExpression(routesProperty.initializer)) {
        routes = routesProperty.initializer;
      }
    }
    typescript.forEachChild(node, visit);
  };
  visit(source);
  if (!routes) throw new Error('createRouter routes array was not found');

  const rows = [];
  const walk = (array, parentPath = '') => {
    for (const element of array.elements) {
      if (!typescript.isObjectLiteralExpression(element)) continue;
      const routePath = literalText(property(element, 'path')?.initializer);
      if (routePath === undefined) continue;
      const fullPath = joinRoutePath(parentPath, routePath);
      const name = literalText(property(element, 'name')?.initializer) ?? '—';
      let pageTitle = '—';
      const meta = property(element, 'meta')?.initializer;
      if (meta && typescript.isObjectLiteralExpression(meta)) {
        pageTitle = literalText(property(meta, 'pageTitle')?.initializer) ?? '—';
      }
      rows.push({ path: fullPath, name, surface: surfaceFor(fullPath), pageTitle });
      const children = property(element, 'children')?.initializer;
      if (children && typescript.isArrayLiteralExpression(children)) walk(children, fullPath);
    }
  };
  walk(routes);
  return rows;
}

function tableCell(value) {
  return String(value).replaceAll('|', '\\|').replaceAll('\n', ' ');
}

export function renderRouteInventory(rows) {
  const lines = [
    '| Path | Route name | Surface | Page title |',
    '| --- | --- | --- | --- |'
  ];
  for (const row of rows) {
    lines.push(
      `| \`${tableCell(row.path)}\` | \`${tableCell(row.name)}\` | ${tableCell(row.surface)} | ${tableCell(row.pageTitle)} |`
    );
  }
  return lines.join('\n');
}

export function replaceGeneratedBlock(document, generated) {
  const start = document.indexOf(START_MARKER);
  const end = document.indexOf(END_MARKER);
  if (start < 0 || end < 0 || end <= start) throw new Error('route inventory marker pair is missing or invalid');
  if (document.indexOf(START_MARKER, start + START_MARKER.length) >= 0 || document.indexOf(END_MARKER, end + END_MARKER.length) >= 0) {
    throw new Error('route inventory markers must appear exactly once');
  }
  const contentStart = start + START_MARKER.length;
  return `${document.slice(0, contentStart)}\n${generated}\n${document.slice(end)}`;
}

function run() {
  const mode = process.argv.slice(2);
  if (mode.length !== 1 || !['--write', '--check'].includes(mode[0])) {
    throw new Error('usage: node scripts/context-sync.mjs --write|--check');
  }
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
  const routerPath = path.join(root, 'src/router/index.ts');
  const documentPath = path.join(root, 'docs/knowledge/01-viewer-admin-flows.md');
  const current = fs.readFileSync(documentPath, 'utf8');
  const routes = extractRouteRows(fs.readFileSync(routerPath, 'utf8'), ts);
  const expected = replaceGeneratedBlock(current, renderRouteInventory(routes));
  if (mode[0] === '--check') {
    if (expected !== current) {
      console.error('context sync failed: route inventory is stale; run npm run context:sync');
      process.exitCode = 1;
      return;
    }
    console.log(`context sync ok: routes=${routes.length}`);
    return;
  }
  if (expected !== current) fs.writeFileSync(documentPath, expected, 'utf8');
  console.log(`context sync updated: routes=${routes.length}`);
}

const invokedPath = process.argv[1] ? path.resolve(process.argv[1]) : '';
if (invokedPath === fileURLToPath(import.meta.url)) run();
