export async function api(path: string) {
  const response = await fetch(path);
  return response.json();
}
