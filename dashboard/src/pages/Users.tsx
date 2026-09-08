import { useEffect, useState } from "react";
import { api } from "../api/client";

export default function Users() {
  const [users, setUsers] = useState<any[]>([]);

  useEffect(() => {
    api.get("/users").then(setUsers).catch(console.error);
  }, []);

  return <pre>{JSON.stringify(users, null, 2)}</pre>;
}
