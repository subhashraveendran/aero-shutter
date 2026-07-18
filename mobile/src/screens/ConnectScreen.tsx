import { useEffect, useRef, useState } from 'react';
import { useStore } from '../store';
import { ApertureIcon, ChevronDown, WifiIcon } from '../components/icons';

const IP_RE = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

/**
 * Local, UI-only phase machine layered over the store's connect lifecycle.
 * The store exposes coarse flags (connecting / connectError / connected); we
 * derive the auto-detect narrative — searching → found → handshaking — for a
 * lively, honest readout. No global state is added.
 */
type Phase = 'idle' | 'searching' | 'found' | 'handshaking';

export function ConnectScreen() {
  const connect = useStore((s) => s.connect);
  const enterDemo = useStore((s) => s.enterDemo);
  const connecting = useStore((s) => s.connecting);
  const connectError = useStore((s) => s.connectError);
  const demo = useStore((s) => s.demo);
  const savedIp = useStore((s) => s.settings.cameraIp);

  const [phase, setPhase] = useState<Phase>('idle');
  const [advanced, setAdvanced] = useState(false);
  const [ip, setIp] = useState(savedIp);
  const [focus, setFocus] = useState(false);
  const timers = useRef<number[]>([]);
  const started = useRef(false);

  const valid = IP_RE.test(ip.trim());

  const clearTimers = () => {
    timers.current.forEach((t) => window.clearTimeout(t));
    timers.current = [];
  };

  // Drive the searching → found → handshaking narrative while the store is
  // busy connecting. These are cosmetic phase labels only.
  useEffect(() => {
    if (connecting) {
      clearTimers();
      setPhase('searching');
      timers.current.push(
        window.setTimeout(() => setPhase('found'), 900),
        window.setTimeout(() => setPhase('handshaking'), 1650),
      );
    } else {
      clearTimers();
      if (connectError) setPhase('idle');
    }
    return clearTimers;
  }, [connecting, connectError]);

  // Auto-detect on first mount — call connect() with NO argument so the store
  // can auto-discover the camera. Manual IP is an escape hatch, not the default.
  // In browser demo mode the mock connects instantly, which would skip this
  // screen entirely; there we wait for the explicit "Try demo mode" button so
  // the auto-detect experience stays visible.
  useEffect(() => {
    if (started.current || demo) return;
    started.current = true;
    void connect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // If auto-detect failed, surface the advanced field automatically.
  useEffect(() => {
    if (connectError) setAdvanced(true);
  }, [connectError]);

  const busy = connecting;
  const detectedIp = ip.trim() && valid ? ip.trim() : savedIp || '192.168.1.1';

  let label: string;
  if (busy) {
    if (phase === 'found') label = `Found camera at ${detectedIp}`;
    else if (phase === 'handshaking') label = 'Opening session…';
    else label = 'Searching for your Nikon…';
  } else if (connectError) {
    label = 'No camera found';
  } else {
    label = 'Standing by';
  }

  return (
    <div className="connect scroll">
      <div className="wordmark">
        <div className="logo">
          <ApertureIcon size={30} />
        </div>
        <h1>
          Aero<span className="amber-text">Shutter</span>
        </h1>
        <span className="tag">Wi-Fi Film Loader</span>
      </div>

      <div
        className={`radar ${busy ? 'live' : ''} ${phase === 'found' || phase === 'handshaking' ? 'locked' : ''} ${connectError ? 'failed' : ''}`}
        role="img"
        aria-label={label}
      >
        <div className="radar-grain" aria-hidden="true" />
        <div className="radar-ring r1" />
        <div className="radar-ring r2" />
        <div className="radar-ring r3" />
        <div className="radar-ticks" />
        <div className="radar-ping p1" />
        <div className="radar-ping p2" />
        <div className="radar-sweep" />
        <div className="radar-core">
          <WifiIcon size={22} className="radar-core-icon" />
        </div>
        {(phase === 'found' || phase === 'handshaking') && <div className="radar-lock" />}
      </div>

      <p className={`searching-label ${connectError ? 'is-error' : ''}`} aria-live="polite">
        <span className="pip" aria-hidden="true" />
        {label}
      </p>

      {!busy && (
        <div className="connect-actions">
          {connectError && (
            <>
              <button className="btn btn-primary btn-block" onClick={() => void connect()}>
                Retry auto-detect
              </button>
              <button
                className="btn btn-ghost btn-block"
                onClick={() => {
                  setAdvanced(true);
                  requestAnimationFrame(() =>
                    document.getElementById('camera-ip-input')?.focus(),
                  );
                }}
              >
                Enter IP manually
              </button>
            </>
          )}
          {demo && (
            <button className="btn btn-ghost btn-block" onClick={() => void enterDemo()}>
              Try demo mode
            </button>
          )}
          {connectError && <p className="error-line">{connectError}</p>}
          {!connectError && (
            <button
              className="btn btn-ghost btn-block connect-manual-link"
              onClick={() => {
                setAdvanced((a) => !a);
                if (!advanced) {
                  requestAnimationFrame(() =>
                    document.getElementById('camera-ip-input')?.focus(),
                  );
                }
              }}
            >
              Connect manually
            </button>
          )}
        </div>
      )}

      <div className="steps">
        <div className="card step crop-marks">
          <div className="num">01</div>
          <div className="step-body">
            <strong>Enable Wi-Fi on the camera</strong>
            <span>On the D5300, open the setup menu and turn on the built-in Wi-Fi.</span>
          </div>
        </div>
        <div className="card step crop-marks">
          <div className="num">02</div>
          <div className="step-body">
            <strong>Join the camera network</strong>
            <span>In phone Wi-Fi settings, connect to “Nikon_WU2_…”.</span>
          </div>
        </div>
        <div className="card step crop-marks">
          <div className="num">03</div>
          <div className="step-body">
            <strong>AeroShutter finds it automatically</strong>
            <span>Auto-detect scans the camera network. Enter an address only if it can’t.</span>
          </div>
        </div>
      </div>

      <button
        className={`advanced-toggle ${advanced ? 'open' : ''}`}
        onClick={() => setAdvanced((a) => !a)}
        aria-expanded={advanced}
      >
        <ChevronDown size={16} className="chev" />
        Advanced — enter IP manually
      </button>

      {advanced && (
        <div className="ip-field">
          <div className="ip-label">Camera Address</div>
          <div className={`ip-wrap ${focus ? 'focus' : ''} ${ip && !valid ? 'invalid' : ''}`}>
            <span className="lead">IP</span>
            <input
              id="camera-ip-input"
              inputMode="decimal"
              value={ip}
              onChange={(e) => setIp(e.target.value)}
              onFocus={() => setFocus(true)}
              onBlur={() => setFocus(false)}
              placeholder="192.168.1.1"
              aria-label="Camera IP address"
            />
            {valid && <span className="valid-dot" aria-hidden="true" />}
          </div>
          <button
            className="btn btn-primary btn-block"
            style={{ marginTop: 'var(--s-3)' }}
            disabled={busy || !valid}
            onClick={() => void connect(ip.trim())}
          >
            {busy ? 'Connecting…' : 'Connect to this address'}
          </button>
        </div>
      )}

      <div className="wifi-note">
        <span className="glyph" aria-hidden="true" />
        <span>
          While joined to the camera’s Wi-Fi, your phone has no internet access. That’s expected —
          reconnect to your normal network when you’re done.
        </span>
      </div>
    </div>
  );
}
