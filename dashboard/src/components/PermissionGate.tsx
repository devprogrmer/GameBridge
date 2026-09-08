type Props = {
  allowed: string[];
  permissions: string[];
  children: React.ReactNode;
};

export default function PermissionGate({ allowed, permissions, children }: Props) {
  const ok = allowed.some((p) => permissions.includes(p));
  return ok ? <>{children}</> : null;
}
