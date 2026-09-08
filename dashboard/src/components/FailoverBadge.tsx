export default function FailoverBadge({enabled}:{enabled:boolean}){
  return <span>{enabled ? "Failover enabled" : "Failover disabled"}</span>;
}
