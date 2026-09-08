export async function api(path:string, options?:RequestInit){
 return fetch(path, options);
}
