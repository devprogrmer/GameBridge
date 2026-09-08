export default function StatusToggle({enabled}:{enabled:boolean}){
  return <button>{enabled ? "Enabled" : "Disabled"}</button>;
}
