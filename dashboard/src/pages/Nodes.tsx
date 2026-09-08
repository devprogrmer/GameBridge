import { useEffect, useState } from "react";
import { api } from "../api/client";

export default function Nodes() {
  const [nodes, setNodes] = useState<any[]>([]);

  useEffect(() => {
    api.get("/nodes").then(setNodes).catch(console.error);
  }, []);

  return <pre>{JSON.stringify(nodes, null, 2)}</pre>;
}
