import { MatchResultPage } from '../MatchResultPage'

interface MatchResultWorkspaceProps {
  bomId: string
  onNavigateToHsResolve?: (model: string, manufacturer: string) => void
}

export function MatchResultWorkspace({
  bomId,
  onNavigateToHsResolve,
}: MatchResultWorkspaceProps) {
  return <MatchResultPage bomId={bomId} onNavigateToHsResolve={onNavigateToHsResolve} />
}
