import { useState } from "react";

const pages = ["Nodes", "Users", "Inbounds", "Outbounds", "Routing", "Settings"];

export default function App() {
  const [page, setPage] = useState("Nodes");

  return (
    <div style={{display:"flex",minHeight:"100vh",fontFamily:"sans-serif"}}>
      <aside style={{width:240,padding:20,borderRight:"1px solid #ddd"}}>
        <h2>GameBridge</h2>
        {pages.map(p => (
          <button
            key={p}
            style={{display:"block",width:"100%",margin:"8px 0",padding:10}}
            onClick={()=>setPage(p)}
          >
            {p}
          </button>
        ))}
      </aside>
      <main style={{padding:30}}>
        <h1>{page}</h1>
        <p>Rebecca-style panel foundation.</p>
      </main>
    </div>
  );
}
