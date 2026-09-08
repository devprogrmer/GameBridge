export default function FormField({label, value, onChange}:{label:string;value:string;onChange:(v:string)=>void}) {
  return <label>{label}<input value={value} onChange={e=>onChange(e.target.value)} /></label>;
}
