export async function apiCreate(path:string, body:unknown){
  return fetch(path,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
}

export async function apiUpdate(path:string, body:unknown){
  return fetch(path,{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
}

export async function apiDelete(path:string){
  return fetch(path,{method:"DELETE"});
}
