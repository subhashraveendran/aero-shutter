import { useEffect, useState } from 'react';
import { useStore } from '../store';
import type { CameraProperty } from '../lib/camera';
import { tap, ImpactStyle } from '../lib/haptics';
import { SlidersIcon } from '../components/icons';

function PropStepper({ prop }: { prop: CameraProperty }) {
  const changeProp = useStore((s) => s.changeProp);
  const [bump, setBump] = useState(false);

  useEffect(() => {
    setBump(true);
    const t = window.setTimeout(() => setBump(false), 240);
    return () => window.clearTimeout(t);
  }, [prop.value]);

  // Find the current value, or snap to the nearest available option so the
  // stepper stays usable even when the value isn't exactly on the step grid.
  let idx = prop.options.indexOf(prop.value);
  if (idx < 0 && prop.options.length > 0) {
    idx = prop.options.reduce(
      (best, v, i) =>
        Math.abs(v - prop.value) < Math.abs(prop.options[best] - prop.value) ? i : best,
      0,
    );
  }
  const canPrev = idx > 0;
  const canNext = idx >= 0 && idx < prop.options.length - 1;

  const step = (dir: number) => {
    const nextIdx = idx + dir;
    if (nextIdx < 0 || nextIdx >= prop.options.length) return;
    void tap();
    void changeProp(prop, prop.options[nextIdx]);
  };

  if (!prop.writable || prop.options.length === 0) {
    return (
      <div className="card prop-card">
        <div className="label">{prop.label}</div>
        <div className="readonly">
          <span className="val">{prop.display}</span>
          <span className="ro-tag">locked</span>
        </div>
      </div>
    );
  }

  return (
    <div className="card prop-card">
      <div className="label">{prop.label}</div>
      <div className={`val ${bump ? 'bump' : ''}`}>{prop.display}</div>
      <div className="stepper">
        <button className="step-btn" disabled={!canPrev} onClick={() => step(-1)} aria-label="Lower">
          −
        </button>
        <div style={{ flex: 1 }} />
        <button className="step-btn" disabled={!canNext} onClick={() => step(1)} aria-label="Raise">
          +
        </button>
      </div>
    </div>
  );
}

export function ControlScreen() {
  const props = useStore((s) => s.props);
  const loading = useStore((s) => s.loadingProps);
  const loadProps = useStore((s) => s.loadProps);
  const capture = useStore((s) => s.capture);
  const cameraModel = useStore((s) => s.cameraModel);

  useEffect(() => {
    if (props.length === 0) void loadProps();
  }, []);

  const noProps = !loading && props.length === 0;

  return (
    <div className="screen">
      <div className="topbar">
        <h1>Exposure</h1>
        <div className="spacer" />
        <span className="readout">
          <span className="r">{cameraModel || 'CAMERA'}</span>
        </span>
      </div>

      <div className="scroll">
        {noProps ? (
          <div className="control-empty">
            <span className="glyph" style={{ margin: '0 auto' }}>
              <SlidersIcon size={30} className="" />
            </span>
            <strong style={{ fontFamily: 'var(--font-display)', fontSize: 19, color: 'var(--text)' }}>
              Controls unavailable
            </strong>
            <span style={{ maxWidth: 280 }}>
              This camera doesn’t expose writable settings over Wi-Fi. You can still browse and
              import frames.
            </span>
          </div>
        ) : (
          <div className="control-list stagger">
            {loading && props.length === 0
              ? Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="card prop-card skeleton" style={{ height: 118 }} />
                ))
              : props.map((prop) => <PropStepper key={prop.code} prop={prop} />)}
          </div>
        )}
      </div>

      {!noProps && (
        <div className="shutter-dock">
          <button
            className="shutter"
            aria-label="Release shutter"
            onClick={() => {
              void tap(ImpactStyle.Heavy);
              void capture();
            }}
          >
            <div className="inner" />
          </button>
          <span className="shutter-label">Release</span>
        </div>
      )}
    </div>
  );
}
