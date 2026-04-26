interface BomWorkbenchPageProps {
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function BomWorkbenchPage({
  onNavigateToHsResolve: _onNavigateToHsResolve,
}: BomWorkbenchPageProps) {
  return (
    <div className="space-y-6" data-testid="bom-workbench-page">
      <div>
        <h2 className="text-2xl font-bold text-slate-800">{'BOM\u5de5\u4f5c\u53f0'}</h2>
        <p className="mt-1 text-sm text-slate-600">
          {'\u96c6\u4e2d\u7ba1\u7406 BOM \u4f1a\u8bdd\u3001\u641c\u7d22\u6e05\u6d17\u3001\u7f3a\u53e3\u5904\u7406\u548c\u5339\u914d\u7ed3\u679c\u3002'}
        </p>
      </div>
    </div>
  )
}
