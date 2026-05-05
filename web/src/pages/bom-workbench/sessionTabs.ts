export type SessionWorkbenchTab =
  | 'overview'
  | 'lines'
  | 'search-tasks'
  | 'data-clean'
  | 'gaps'
  | 'maintenance'
  | 'match'

export const SESSION_WORKBENCH_TABS: { id: SessionWorkbenchTab; label: string }[] = [
  { id: 'overview', label: '\u6982\u89c8' },
  { id: 'lines', label: 'BOM\u884c' },
  { id: 'search-tasks', label: '\u4efb\u52a1\u7ba1\u7406' },
  { id: 'data-clean', label: '\u6570\u636e\u6e05\u6d17' },
  { id: 'gaps', label: '\u7f3a\u53e3\u5904\u7406' },
  { id: 'maintenance', label: '\u7ef4\u62a4' },
  { id: 'match', label: '\u5339\u914d\u7ed3\u679c' },
]
