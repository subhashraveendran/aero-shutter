import { useEffect, useRef, useState } from 'react';
import { useStore, type ImportTask } from '../store';
import { formatBytes, formatEta, formatRate } from '../lib/format';
import { CheckIcon, RefreshIcon, XIcon } from '../components/icons';

function StatusGlyph({ status }: { status: ImportTask['status'] }) {
  if (status === 'done')
    return (
      <span className="ok">
        <CheckIcon size={16} className="" />
      </span>
    );
  if (status === 'error')
    return (
      <span className="err">
        <XIcon size={16} className="" />
      </span>
    );
  if (status === 'active')
    return (
      <span className="spin">
        <RefreshIcon size={16} className="" />
      </span>
    );
  return <span className="queued" />;
}

function useRate(task: ImportTask): { rate: number; eta: number } {
  const prev = useRef<{ t: number; bytes: number } | null>(null);
  const [rate, setRate] = useState(0);

  useEffect(() => {
    if (task.status !== 'active') {
      prev.current = null;
      return;
    }
    const now = performance.now();
    if (prev.current) {
      const dt = (now - prev.current.t) / 1000;
      const db = task.bytesDone - prev.current.bytes;
      if (dt > 0.05) setRate((r) => r * 0.6 + (db / dt) * 0.4);
    }
    prev.current = { t: now, bytes: task.bytesDone };
  }, [task.bytesDone, task.status]);

  const remaining = task.totalBytes - task.bytesDone;
  const eta = rate > 0 ? remaining / rate : Infinity;
  return { rate, eta };
}

function QueueItem({ task }: { task: ImportTask }) {
  const { rate, eta } = useRate(task);
  const pct = task.totalBytes > 0 ? (task.bytesDone / task.totalBytes) * 100 : 0;

  return (
    <div className={`card qitem ${task.status}`}>
      <div className="qitem-head">
        <span className="glyph">
          <StatusGlyph status={task.status} />
        </span>
        <span className="name">{task.photo.filename}</span>
        <span className={`qitem-status ${task.status}`}>
          {task.status === 'done'
            ? 'DONE'
            : task.status === 'error'
              ? 'ERROR'
              : task.status === 'active'
                ? `${pct.toFixed(0)}%`
                : 'QUEUED'}
        </span>
      </div>
      <div className="bar">
        <span style={{ width: `${task.status === 'done' ? 100 : pct}%` }} />
      </div>
      <div className="qitem-meta">
        <span>
          {formatBytes(task.bytesDone)} / {formatBytes(task.totalBytes)}
        </span>
        <span>
          {task.status === 'active' && rate > 0
            ? `${formatRate(rate)} · ${formatEta(eta)}`
            : task.status === 'error'
              ? task.error
              : ''}
        </span>
      </div>
    </div>
  );
}

export function ImportScreen() {
  const queue = useStore((s) => s.importQueue);
  const importing = useStore((s) => s.importing);
  const importedToday = useStore((s) => s.importedTodayCount);
  const navigate = useStore((s) => s.navigate);
  const cancelImport = useStore((s) => s.cancelImport);

  const done = queue.filter((t) => t.status === 'done').length;
  const errors = queue.filter((t) => t.status === 'error').length;
  const overall =
    queue.length > 0
      ? (queue.reduce(
          (acc, t) =>
            acc + (t.status === 'done' ? 1 : t.totalBytes > 0 ? t.bytesDone / t.totalBytes : 0),
          0,
        ) /
          queue.length) *
        100
      : 0;

  return (
    <div className="screen">
      <div className="topbar">
        <h1>Developing</h1>
        <div className="spacer" />
        {importing ? (
          <button className="badge badge-demo" onClick={() => void cancelImport()}>
            <XIcon size={12} className="" /> Cancel
          </button>
        ) : (
          <button className="badge badge-demo" onClick={() => navigate('gallery')}>
            <CheckIcon size={12} className="" /> Done
          </button>
        )}
      </div>

      <div className="scroll">
        <div className="card queue-summary">
          <div>
            <div className="big-num">
              {String(done).padStart(2, '0')}/{String(queue.length || 0).padStart(2, '0')}
            </div>
            <div className="sub">
              {importing ? 'Developing…' : errors > 0 ? `${errors} failed` : 'Complete'}
            </div>
          </div>
          <div style={{ flex: 1 }} />
          <div style={{ textAlign: 'right' }}>
            <div className="big-num muted">{String(importedToday).padStart(2, '0')}</div>
            <div className="sub">today</div>
          </div>
        </div>

        {queue.length > 0 && (
          <div className="develop-bar">
            <span style={{ width: `${overall}%` }} />
          </div>
        )}

        {queue.length === 0 ? (
          <div className="empty">
            <span className="glyph">
              <CheckIcon size={28} className="" />
            </span>
            <strong>Tray is empty</strong>
            <span>Select frames in the contact sheet to develop them.</span>
          </div>
        ) : (
          <div className="queue-list">
            {queue.map((task) => (
              <QueueItem key={task.photo.handle} task={task} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
