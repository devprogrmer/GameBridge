import { useState } from "react";
import { api } from "../api/client";

export default function Login() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  async function login() {
    await api.post("/auth/login", { username, password });
    location.href = "/";
  }

  return (
    <div>
      <h1>GameBridge Login</h1>
      <input placeholder="username" onChange={e => setUsername(e.target.value)} />
      <input placeholder="password" type="password" onChange={e => setPassword(e.target.value)} />
      <button onClick={login}>Login</button>
    </div>
  );
}
