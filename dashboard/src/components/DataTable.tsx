export default function DataTable({title, rows}:{title:string; rows:any[]}) {
  return (
    <div>
      <h2>{title}</h2>
      <pre>{JSON.stringify(rows, null, 2)}</pre>
    </div>
  );
}
