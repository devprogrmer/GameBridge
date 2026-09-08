export async function getSession() {
  const res = await fetch('/api/auth/session', { credentials: 'include' });
  if (!res.ok) throw new Error('session unavailable');
  return res.json();
}

export async function logout() {
  await fetch('/api/auth/logout', {
    method: 'POST',
    credentials: 'include',
  });
}
