const API = "/api";

async function request(path: string, options: RequestInit = {}) {
  const res = await fetch(`${API}${path}`, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });

  if (!res.ok) {
    throw new Error(await res.text());
  }

  return res.json();
}

export const api = {
  get: (path: string) => request(path),
  post: (path: string, body: unknown) =>
    request(path, {
      method: "POST",
      body: JSON.stringify(body),
    }),
};
