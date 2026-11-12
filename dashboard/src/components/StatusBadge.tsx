import Badge from 'react-bootstrap/Badge';
import { STATUS_LABEL, STATUS_VARIANT } from '../labels';

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const variant = STATUS_VARIANT[status] ?? 'secondary';
  return (
    <Badge bg={variant} text={variant === 'warning' || variant === 'info' ? 'dark' : undefined}>
      {label ?? STATUS_LABEL[status] ?? status}
    </Badge>
  );
}

/** Nom de rôle avec sa couleur (app.css). */
export function RoleTag({ role }: { role: string }) {
  return <span className="role-tag font-monospace" data-role={role}>{role}</span>;
}
