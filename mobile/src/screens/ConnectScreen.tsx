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

const NIKON_SSID_PREFIX = 'Nikon_WU2_';

export function ConnectScreen() {
  const connect = useStore((s) => s.connect);
  const enterDemo = useStore((s) => s.enterDemo);
  const joinCameraWifi = useStore((s) => s.joinCameraWifi);
  const connecting = useStore((s) => s.connecting);
  const connectError = useStore((s) => s.connectError);
  const joiningWifi = useStore((s) => s.joiningWifi);
  const wifiError = useStore((s) => s.wifiError);
  const wifiSsid = useStore((s) => s.wifiSsid);
  const demo = useStore((s) => s.demo);
  const savedIp = useStore((s) => s.settings.cameraIp);

  const [phase, setPhase] = useState<Phase>('idle');
  // A SINGLE progressive-disclosure panel holds every technical option (Wi-Fi
  // name/password + manual IP). It stays hidden so the default screen is just
  // one button; it opens on demand or automatically when a connect fails.
  const [helpOpen, setHelpOpen] = useState(false);
  const [ssid, setSsid] = useState(NIKON_SSID_PREFIX);
  const [password, setPassword] = useState('');
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
  // can auto-discover the camera. The technical options are an escape hatch,
  // not the default. In browser demo mode the mock connects instantly, which
  // would skip this screen entirely; there we wait for the explicit "Try demo
  // mode" button so the auto-detect experience stays visible.
  useEffect(() => {
    if (started.current || demo) return;
    started.current = true;
    void connect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // If auto-detect failed, reveal the options panel so the fix is one tap away.
  useEffect(() => {
    if (connectError || wifiError) setHelpOpen(true);
  }, [connectError, wifiError]);

  const busy = connecting || joiningWifi;
  const detectedIp = ip.trim() && valid ? ip.trim() : savedIp || '192.168.1.1';

  const ssidTrimmed = ssid.trim();
  const canJoin = ssidTrimmed.length > 0;
  const doJoin = () => void joinCameraWifi(ssidTrimmed || NIKON_SSID_PREFIX, password);

  let label: string;
  if (joiningWifi) {
    label = 'Joining camera Wi-Fi…';
  } else if (connecting) {
    if (phase === 'found') label = `Found camera at ${detectedIp}`;
    else if (phase === 'handshaking') label = 'Opening session…';
    else label = 'Searching for your Nikon…';
  } else if (connectError || wifiError) {
    label = wifiError && !connectError ? 'Could not join Wi-Fi' : 'No camera found';
  } else if (wifiSsid) {
    label = `Joined ${wifiSsid}`;
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

      <p className={`searching-label ${connectError || wifiError ? 'is-error' : ''}`} aria-live="polite">
        <span className="pip" aria-hidden="true" />
        {label}
      </p>

      {wifiSsid && !busy && (
        <p className="wifi-joined" aria-live="polite">
          <WifiIcon size={13} /> Connected to {wifiSsid}
        </p>
      )}

      {/* One plain-language line so a first-timer knows the single prerequisite. */}
      {!busy && !wifiSsid && (
        <p className="connect-hint">
          Turn on Wi-Fi in your camera’s menu, then tap below — AeroShutter finds it for you.
        </p>
      )}

      {!busy && (
        <div className="connect-actions">
          {/* THE one primary action. It joins the camera's "Nikon_WU2_…" network
              and auto-connects. Everything else is tucked into "Trouble
              connecting?" below so this screen stays a single clear choice. */}
          <button className="btn btn-primary btn-block join-wifi-btn" onClick={doJoin}>
            <WifiIcon size={18} />
            {connectError || wifiError ? 'Try again' : 'Connect to camera'}
          </button>

          {demo && (
            <button className="btn btn-ghost btn-block" onClick={() => void enterDemo()}>
              Try demo mode
            </button>
          )}

          {/* Single disclosure for ALL technical options. Hidden by default,
              auto-opened on error. */}
          <button
            className="btn btn-ghost btn-block connect-manual-link"
            onClick={() => setHelpOpen((o) => !o)}
            aria-expanded={helpOpen}
          >
            <ChevronDown size={15} className={`chev ${helpOpen ? 'open' : ''}`} />
            Trouble connecting?
          </button>

          {helpOpen && (
            <div className="connect-help">
              {(connectError || wifiError) && (
                <p className="error-line">{connectError || wifiError}</p>
              )}

              {/* Option A — enter the camera's exact Wi-Fi name / password. */}
              <div className="wifi-field">
                <div className="ip-label">Camera Wi-Fi name</div>
                <div className="ip-wrap">
                  <span className="lead">WiFi</span>
                  <input
                    value={ssid}
                    onChange={(e) => setSsid(e.target.value)}
                    placeholder="Nikon_WU2_XXXXXX"
                    aria-label="Camera Wi-Fi SSID"
                    autoCapitalize="none"
                    autoCorrect="off"
                  />
                </div>
                <div className="ip-label" style={{ marginTop: 'var(--s-3)' }}>
                  Password (leave blank if none)
                </div>
                <div className="ip-wrap">
                  <span className="lead">Key</span>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="optional"
                    aria-label="Camera Wi-Fi password"
                    autoCapitalize="none"
                    autoCorrect="off"
                  />
                </div>
                <button
                  className="btn btn-primary btn-block"
                  style={{ marginTop: 'var(--s-3)' }}
                  disabled={!canJoin}
                  onClick={doJoin}
                >
                  Join &amp; connect
                </button>
              </div>

              {/* Option B — already on the camera's Wi-Fi? enter its address. */}
              <div className="ip-field" style={{ marginTop: 'var(--s-4)' }}>
                <div className="ip-label">Already joined? Enter the camera’s address</div>
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
                  className="btn btn-ghost btn-block"
                  style={{ marginTop: 'var(--s-3)' }}
                  disabled={busy || !valid}
                  onClick={() => void connect(ip.trim())}
                >
                  {busy ? 'Connecting…' : 'Connect to this address'}
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      <div className="wifi-note">
        <span className="glyph" aria-hidden="true" />
        <span>
          While you’re on the camera’s Wi-Fi, your phone has no internet — that’s normal. It comes
          back when you disconnect.
        </span>
      </div>
    </div>
  );
}
