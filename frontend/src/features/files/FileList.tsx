import { fileDownloadURL, type FileInfo } from '../../api'

export function FileList({ files }: { files: FileInfo[] }) {
  return (
    <>
      <h2 className="section-title">Files ({files.length})</h2>
      {files.length === 0 && <p className="muted pad">No files yet. Select a peer, then attach a file below.</p>}
      <ul className="bundle-list">
        {files.map((f) => (
          <li key={f.id} className="bundle-item">
            <strong>{f.name}</strong>
            <span className="mono">
              {f.mine ? 'sent' : `from ${f.from_name}`} · {f.size} bytes
            </span>
            <a className="download" href={fileDownloadURL(f.id)}>
              Download
            </a>
          </li>
        ))}
      </ul>
    </>
  )
}
