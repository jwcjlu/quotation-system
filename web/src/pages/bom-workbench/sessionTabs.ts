export type SessionWorkbenchTab =
  | 'overview'
  | 'lines'
  | 'search-clean'
  | 'gaps'
  | 'maintenance'
  | 'match'

export const SESSION_WORKBENCH_TABS: { id: SessionWorkbenchTab; label: string }[] = [
  { id: 'overview', label: '\u6982\u89c8' },
  { id: 'lines', label: 'BOM\u884c' },
  { id: 'search-clean', label: '\u641c\u7d22\u6e05\u6d17' },
  { id: 'gaps', label: '\u7f3a\u53e3\u5904\u7406' },
  { id: 'maintenance', label: '\u7ef4\u62a4' },
  { id: 'match', label: '\u5339\u914d\u7ed3\u679c' },
]
