export default function MaintenanceBadge({enabled}:{enabled:boolean}){
  return <span>{enabled ? "Maintenance mode" : "Normal mode"}</span>;
}
