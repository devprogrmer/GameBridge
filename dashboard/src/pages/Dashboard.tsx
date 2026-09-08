import { useEffect, useState } from "react";
import { api } from "../api/client";

export default function Dashboard() {
  const [data, setData] = useState<any>(null);

  useEffect(() => {
    api.get("/dashboard").then(setData).catch(console.error);
  }, []);

  return (
    <div>
      <h1>Dashboard</h1>
      <pre>{JSON.stringify(data, null, 2)}</pre>
    </div>
  );
}
