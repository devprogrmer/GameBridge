export async function apiGet(path:string){
  const res = await fetch(path, {credentials:"include"});
  if(!res.ok) throw new Error("request failed");
  return res.json();
}
