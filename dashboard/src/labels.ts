// Libellés et couleurs Bootstrap des statuts.

export const STATUS_LABEL: Record<string, string> = {
  todo: 'à faire', running: 'en cours', review: 'en vérification', done: 'terminée', failed: 'échec',
  planning: 'planification', awaiting_approval: 'attend une approbation', cancelled: 'annulé',
  pending: 'en attente', approved: 'approuvée', rejected: 'refusée',
  succeeded: 'réussi', killed: 'arrêté', passed: 'réussi', error: 'erreur',
};

/** Couleur Bootstrap (bg d'un Badge, bordure d'une Card) pour chaque statut. */
export const STATUS_VARIANT: Record<string, string> = {
  todo: 'secondary', planning: 'primary', running: 'primary', review: 'info',
  awaiting_approval: 'warning', pending: 'warning',
  done: 'success', succeeded: 'success', approved: 'success', passed: 'success',
  failed: 'danger', rejected: 'danger', killed: 'danger', cancelled: 'danger', error: 'danger',
};

export const STATUS_ICON: Record<string, string> = { todo: '○', running: '◐', review: '◎', done: '✓', failed: '✕' };

export const KIND_LABEL: Record<string, string> = { plan: 'planification', task: 'tâche' };

export const APPROVAL_LABEL: Record<string, string> = { merge: 'Merge' };

export function duration(ms: number): string {
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${String(s % 60).padStart(2, '0')}`;
}

export const time = (iso: string) => new Date(iso).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
export const shortTime = (iso: string) => new Date(iso).toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
