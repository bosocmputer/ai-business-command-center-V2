# Nextstep Dashboard Frontend

Vue 3, TypeScript, PrimeVue, and the Sakai application shell for the Nextstep
admin and LINE LIFF viewer experiences.

## Local development

Requirements: Node.js 22+ and the backend listening on `127.0.0.1:8080`.

```bash
cp .env.example .env.local
npm ci
npm run dev
```

The Vite development server proxies `/api` to the backend so browser cookies
remain same-origin. `VITE_LINE_LIFF_ID` is a public LIFF identifier, not a
secret.

Production images must receive the dashboard LIFF ID from the release
configuration (`vars.VITE_LINE_LIFF_ID`). Never infer it from a compiled bundle:
the LINE SDK contains its own internal LIFF IDs, and the build guard rejects
those IDs before an image can be published.

## API contract

The backend owns the OpenAPI contract. Regenerate the client types after an API
change and include the generated diff in the same change:

```bash
npm run openapi:generate
```

Generation also updates `api/contract.json` with the full backend commit SHA and
the generated schema checksum. CI runs `npm run contract:verify`; deploy only a
frontend/backend SHA pair whose contract file points to that backend commit.

## Verification

```bash
npm run typecheck
npm run contract:verify
npm run lint
npm test
npm run test:e2e
npm run build
npm audit --audit-level=high
```

Production builds do not publish source maps. Hashed assets are cached for one
year, while the SPA entry point is never cached.
