# BOM LLM Async Import Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the frontend flow for async `UploadBOM(parse_mode=llm)` so upload jumps straight to the sourcing session page and that page polls and displays import progress until `ready` or `failed`.

**Architecture:** Extend the existing API parsing layer to surface async import fields, keep `UploadPage` responsible only for starting the upload and redirecting, and make `SourcingSessionPage` the single place that owns import-progress polling and progress UI. Because the current web app has no frontend test runner, add a minimal Vitest + React Testing Library setup first, then drive the behavior changes with failing tests.

**Tech Stack:** React 19, TypeScript, Vite, Vitest, React Testing Library

---

## File Map

- **Modify** `web/package.json`
  - Add frontend test dependencies and scripts.
- **Create** `web/vitest.config.ts`
  - Vitest config for jsdom + setup file.
- **Create** `web/src/test/setup.ts`
  - Shared testing setup and cleanup hooks.
- **Create** `web/src/api/bomLegacy.test.ts`
  - Tests for `uploadBOM` async response parsing.
- **Create** `web/src/api/bomSession.test.ts`
  - Tests for `getSession` parsing of import progress fields.
- **Modify** `web/src/api/types.ts`
  - Add async import fields to TypeScript interfaces.
- **Modify** `web/src/api/bomLegacy.ts`
  - Parse `accepted/import_status/import_message`.
- **Modify** `web/src/api/bomSession.ts`
  - Parse session import fields.
- **Create** `web/src/pages/UploadPage.test.tsx`
  - Tests for immediate redirect behavior on accepted LLM upload.
- **Create** `web/src/pages/SourcingSessionPage.test.tsx`
  - Tests for progress card rendering and polling stop conditions.
- **Modify** `web/src/pages/UploadPage.tsx`
  - Keep non-LLM sync flow intact and redirect immediately for accepted LLM uploads.
- **Modify** `web/src/pages/SourcingSessionPage.tsx`
  - Add import progress card, polling, and action gating.

---

### Task 1: Add Frontend Test Infrastructure

**Files:**
- Modify: `web/package.json`
- Create: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`

- [ ] **Step 1: Write the failing tooling expectation**

Create a placeholder API test file so the first `npm test` fails because the test script and Vitest setup do not exist yet.

```ts
// web/src/api/bomLegacy.test.ts
import { describe, expect, it } from 'vitest'

describe('placeholder', () => {
  it('runs vitest', () => {
    expect(true).toBe(true)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand`

Expected: FAIL because `npm test` script is missing.

- [ ] **Step 3: Add the minimal test runner setup**

Update `web/package.json`:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "test": "vitest run"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.8.0",
    "@testing-library/react": "^16.1.0",
    "@testing-library/user-event": "^14.6.1",
    "jsdom": "^26.1.0",
    "vitest": "^2.1.8"
  }
}
```

Create `web/vitest.config.ts`:

```ts
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: [fileURLToPath(new URL('./src/test/setup.ts', import.meta.url))],
    css: true,
  },
})
```

Create `web/src/test/setup.ts`:

```ts
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

afterEach(() => {
  cleanup()
})
```

- [ ] **Step 4: Install and run tests to verify the runner works**

Run: `npm test`

Expected: PASS for the placeholder test.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test/setup.ts web/src/api/bomLegacy.test.ts
git commit -m "test(web): add vitest frontend test setup"
```

---

### Task 2: Extend API Parsing for Async Import Fields

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/api/bomLegacy.ts`
- Modify: `web/src/api/bomSession.ts`
- Modify: `web/src/api/bomLegacy.test.ts`
- Create: `web/src/api/bomSession.test.ts`

- [ ] **Step 1: Write the failing API parsing tests**

Replace the placeholder test in `web/src/api/bomLegacy.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'

vi.mock('./http', () => ({
  fetchJson: vi.fn(async () => ({
    bom_id: 'session-1',
    accepted: true,
    import_status: 'parsing',
    import_message: 'import started',
    items: [],
    total: 0,
  })),
}))

describe('uploadBOM', () => {
  it('parses async import response fields', async () => {
    const file = new File(['bom'], 'bom.xlsx')
    const { uploadBOM } = await import('./bomLegacy')

    const result = await uploadBOM(file, 'llm', undefined, { sessionId: 'session-1' })

    expect(result.accepted).toBe(true)
    expect(result.import_status).toBe('parsing')
    expect(result.import_message).toBe('import started')
  })
})
```

Create `web/src/api/bomSession.test.ts`:

```ts
import { describe, expect, it, vi } from 'vitest'

vi.mock('./http', () => ({
  fetchJson: vi.fn(async () => ({
    session_id: 'session-1',
    title: 'Session 1',
    status: 'draft',
    biz_date: '2026-04-21',
    selection_revision: 1,
    platform_ids: ['digikey'],
    import_status: 'parsing',
    import_progress: 35,
    import_stage: 'chunk_parsing',
    import_message: 'chunk 1/3',
    import_error_code: '',
    import_error: '',
    import_updated_at: '2026-04-21T12:00:00Z',
  })),
}))

describe('getSession', () => {
  it('parses import progress fields', async () => {
    const { getSession } = await import('./bomSession')
    const result = await getSession('session-1')

    expect(result.import_status).toBe('parsing')
    expect(result.import_progress).toBe(35)
    expect(result.import_stage).toBe('chunk_parsing')
    expect(result.import_message).toBe('chunk 1/3')
    expect(result.import_updated_at).toBe('2026-04-21T12:00:00Z')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm test -- web/src/api/bomLegacy.test.ts web/src/api/bomSession.test.ts`

Expected: FAIL because the return types and parsers do not expose the new fields.

- [ ] **Step 3: Implement the minimal parsing changes**

Update `web/src/api/types.ts`:

```ts
export interface GetSessionReply {
  session_id: string
  title: string
  status: string
  biz_date: string
  selection_revision: number
  platform_ids: string[]
  customer_name?: string
  contact_phone?: string
  contact_email?: string
  contact_extra?: string
  readiness_mode?: string
  import_status?: string
  import_progress?: number
  import_stage?: string
  import_message?: string
  import_error_code?: string
  import_error?: string
  import_updated_at?: string
}
```

Update the `uploadBOM` return type in `web/src/api/bomLegacy.ts`:

```ts
): Promise<{
  bom_id: string
  items: ParsedItem[]
  total: number
  accepted: boolean
  import_status: string
  import_message: string
}> {
```

And the returned object:

```ts
  return {
    bom_id: (json.bom_id ?? json.bomId) as string,
    items: (json.items ?? []) as ParsedItem[],
    total: Number(json.total ?? 0),
    accepted: Boolean(json.accepted),
    import_status: String(json.import_status ?? json.importStatus ?? ''),
    import_message: String(json.import_message ?? json.importMessage ?? ''),
  }
```

Update `parseGetSession` in `web/src/api/bomSession.ts`:

```ts
    import_status: str(json.import_status ?? json.importStatus) || undefined,
    import_progress: num(json.import_progress ?? json.importProgress, 0),
    import_stage: str(json.import_stage ?? json.importStage) || undefined,
    import_message: str(json.import_message ?? json.importMessage) || undefined,
    import_error_code: str(json.import_error_code ?? json.importErrorCode) || undefined,
    import_error: str(json.import_error ?? json.importError) || undefined,
    import_updated_at: str(json.import_updated_at ?? json.importUpdatedAt) || undefined,
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm test -- web/src/api/bomLegacy.test.ts web/src/api/bomSession.test.ts`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/bomLegacy.ts web/src/api/bomSession.ts web/src/api/bomLegacy.test.ts web/src/api/bomSession.test.ts
git commit -m "feat(web): parse async bom import API fields"
```

---

### Task 3: Make Upload Page Redirect Immediately for Accepted LLM Imports

**Files:**
- Modify: `web/src/pages/UploadPage.tsx`
- Create: `web/src/pages/UploadPage.test.tsx`

- [ ] **Step 1: Write the failing upload-page tests**

Create `web/src/pages/UploadPage.test.tsx`:

```tsx
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { UploadPage } from './UploadPage'

const createSession = vi.fn()
const uploadBOM = vi.fn()

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    PLATFORM_IDS: ['digikey'],
    createSession,
    uploadBOM,
    downloadTemplate: vi.fn(),
  }
})

describe('UploadPage', () => {
  it('redirects immediately after accepted llm upload', async () => {
    createSession.mockResolvedValue({ session_id: 'session-1' })
    uploadBOM.mockResolvedValue({
      bom_id: 'session-1',
      accepted: true,
      import_status: 'parsing',
      import_message: 'import started',
      items: [],
      total: 0,
    })
    const onSuccess = vi.fn()

    render(<UploadPage onSuccess={onSuccess} />)

    const file = new File(['bom'], 'bom.xlsx', {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    })
    fireEvent.change(screen.getByLabelText(/file-input/i), {
      target: { files: [file] },
    })
    fireEvent.click(screen.getByRole('button', { name: /上传并解析|upload/i }))

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith('session-1')
    })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web/src/pages/UploadPage.test.tsx`

Expected: FAIL due to missing or mismatched async-upload handling in the page.

- [ ] **Step 3: Write the minimal upload-page implementation**

In `web/src/pages/UploadPage.tsx`, update the upload-success branch:

```ts
      const res = await uploadBOM(file, parseMode, mapping, { sessionId: sess.session_id })
      if (!res.bom_id) {
        throw new Error('upload succeeded without bom_id')
      }
      if (parseMode === 'llm' && !res.accepted) {
        throw new Error(res.import_message || 'llm import was not accepted')
      }
      onSuccess(res.bom_id)
```

If the file-input `label` query is too brittle in the test, add a stable test id:

```tsx
<input
  data-testid="upload-file-input"
  type="file"
  accept=".xlsx,.xls"
```
```

and then use:

```tsx
fireEvent.change(screen.getByTestId('upload-file-input'), {
  target: { files: [file] },
})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm test -- web/src/pages/UploadPage.test.tsx`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/UploadPage.tsx web/src/pages/UploadPage.test.tsx
git commit -m "feat(web): redirect to session page after accepted llm upload"
```

---

### Task 4: Add Import Progress Polling to Sourcing Session Page

**Files:**
- Modify: `web/src/pages/SourcingSessionPage.tsx`
- Create: `web/src/pages/SourcingSessionPage.test.tsx`

- [ ] **Step 1: Write the failing sourcing-session tests**

Create `web/src/pages/SourcingSessionPage.test.tsx`:

```tsx
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SourcingSessionPage } from './SourcingSessionPage'

vi.useFakeTimers()

const getSession = vi.fn()
const getBOMLines = vi.fn()
const getSessionSearchTaskCoverage = vi.fn()

vi.mock('../api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('../api')
  return {
    ...actual,
    PLATFORM_IDS: ['digikey'],
    getSession,
    getBOMLines,
    getSessionSearchTaskCoverage,
    patchSession: vi.fn(),
    putPlatforms: vi.fn(),
    createSessionLine: vi.fn(),
    patchSessionLine: vi.fn(),
    deleteSessionLine: vi.fn(),
    retrySearchTasks: vi.fn(),
    exportSessionFile: vi.fn(),
  }
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('SourcingSessionPage import progress', () => {
  it('polls while parsing and stops after ready', async () => {
    getSession
      .mockResolvedValueOnce({
        session_id: 'session-1',
        title: 'Session 1',
        status: 'draft',
        biz_date: '2026-04-21',
        selection_revision: 1,
        platform_ids: ['digikey'],
        import_status: 'parsing',
        import_progress: 35,
        import_stage: 'chunk_parsing',
        import_message: 'chunk 1/3',
      })
      .mockResolvedValueOnce({
        session_id: 'session-1',
        title: 'Session 1',
        status: 'draft',
        biz_date: '2026-04-21',
        selection_revision: 1,
        platform_ids: ['digikey'],
        import_status: 'ready',
        import_progress: 100,
        import_stage: 'done',
        import_message: 'import completed',
      })
    getBOMLines.mockResolvedValue({ lines: [] })
    getSessionSearchTaskCoverage.mockResolvedValue({
      consistent: true,
      orphan_task_count: 0,
      expected_task_count: 0,
      existing_task_count: 0,
      missing_tasks: [],
    })

    render(<SourcingSessionPage sessionId="session-1" />)

    await screen.findByText(/chunk 1\/3/i)
    vi.advanceTimersByTime(2000)

    await waitFor(() => {
      expect(screen.getByText(/import completed/i)).toBeInTheDocument()
    })
  })

  it('shows import failure details and stops polling', async () => {
    getSession.mockResolvedValue({
      session_id: 'session-1',
      title: 'Session 1',
      status: 'draft',
      biz_date: '2026-04-21',
      selection_revision: 1,
      platform_ids: ['digikey'],
      import_status: 'failed',
      import_progress: 0,
      import_stage: 'failed',
      import_message: 'import failed',
      import_error_code: 'BOM_LLM_LIMIT',
      import_error: 'file too large',
    })
    getBOMLines.mockResolvedValue({ lines: [] })
    getSessionSearchTaskCoverage.mockResolvedValue({
      consistent: true,
      orphan_task_count: 0,
      expected_task_count: 0,
      existing_task_count: 0,
      missing_tasks: [],
    })

    render(<SourcingSessionPage sessionId="session-1" />)

    await screen.findByText(/file too large/i)
    expect(screen.getByText(/BOM_LLM_LIMIT/i)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- web/src/pages/SourcingSessionPage.test.tsx`

Expected: FAIL because the page does not yet render import progress or poll.

- [ ] **Step 3: Implement the minimal sourcing-session polling UI**

In `web/src/pages/SourcingSessionPage.tsx`, add local import state:

```ts
  const [importStatus, setImportStatus] = useState('')
  const [importProgress, setImportProgress] = useState(0)
  const [importStage, setImportStage] = useState('')
  const [importMessage, setImportMessage] = useState('')
  const [importErrorCode, setImportErrorCode] = useState('')
  const [importError, setImportError] = useState('')
  const [importUpdatedAt, setImportUpdatedAt] = useState('')
```

Update `loadSession`:

```ts
      setImportStatus((s.import_status || '').trim())
      setImportProgress(Number(s.import_progress || 0))
      setImportStage((s.import_stage || '').trim())
      setImportMessage((s.import_message || '').trim())
      setImportErrorCode((s.import_error_code || '').trim())
      setImportError((s.import_error || '').trim())
      setImportUpdatedAt((s.import_updated_at || '').trim())
```

Add polling:

```ts
  useEffect(() => {
    if (importStatus !== 'parsing') return
    const timer = window.setInterval(() => {
      void loadSession()
    }, 2000)
    return () => window.clearInterval(timer)
  }, [importStatus, loadSession])
```

Add a one-time refresh when parsing completes:

```ts
  useEffect(() => {
    if (importStatus === 'ready') {
      void loadLines()
    }
  }, [importStatus, loadLines])
```

Add a progress card above the editable session sections:

```tsx
      {(importStatus || '').trim() && (
        <section className="rounded-xl border border-sky-200 bg-sky-50 p-4 shadow-sm">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="font-semibold text-slate-800">导入进度</h3>
              <p className="text-sm text-slate-600">
                {importMessage || importStage || importStatus}
              </p>
            </div>
            <div className="text-sm font-medium text-slate-700">{importProgress}%</div>
          </div>
          <div className="mt-3 h-2 rounded-full bg-slate-200">
            <div
              className="h-2 rounded-full bg-sky-500 transition-all"
              style={{ width: `${Math.max(0, Math.min(100, importProgress))}%` }}
            />
          </div>
          {importErrorCode || importError ? (
            <div className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
              <div>{importErrorCode || 'IMPORT_FAILED'}</div>
              {importError && <div className="mt-1">{importError}</div>}
            </div>
          ) : null}
        </section>
      )}
```

Gate actions:

```ts
  const importParsing = importStatus === 'parsing'
```

and apply `disabled={importParsing || !canEnterMatch}` to the match-entry button.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm test -- web/src/pages/SourcingSessionPage.test.tsx`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/SourcingSessionPage.tsx web/src/pages/SourcingSessionPage.test.tsx
git commit -m "feat(web): show async bom import progress on session page"
```

---

### Task 5: Final Verification

**Files:**
- Modify/Test as needed across `web/src/api` and `web/src/pages`

- [ ] **Step 1: Run the frontend test suite**

Run: `npm test`

Expected: PASS

- [ ] **Step 2: Run the frontend production build**

Run: `npm run build`

Expected: PASS

- [ ] **Step 3: Sanity-check the user flow manually**

Verify:
- `llm` upload redirects immediately to the session page
- session page shows parsing progress while polling
- progress stops at `ready`
- error details appear at `failed`
- non-LLM upload still works

- [ ] **Step 4: Commit**

```bash
git add web/src/api web/src/pages web/package.json web/package-lock.json web/vitest.config.ts web/src/test/setup.ts
git commit -m "test(web): verify async bom import frontend flow"
```

