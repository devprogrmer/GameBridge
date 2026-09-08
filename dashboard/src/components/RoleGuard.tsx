type Props = {
  roles: string[];
  role: string;
  children: React.ReactNode;
};

export default function RoleGuard({ roles, role, children }: Props) {
  return roles.includes(role) ? <>{children}</> : null;
}
