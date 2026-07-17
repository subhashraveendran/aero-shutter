import { useState } from 'react';
import { useStore } from '../store';
import { ApertureIcon } from '../components/icons';

const IP_RE = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

export function ConnectScreen() {
  const connect = useStore((s) => s.connect);
  const enterDemo = useStore((s) => s.enterDemo);
  const connecting = useStore((s) => s.connecting);
  const connectError = useStore((s) => s.connectError);
  const demo = useStore((s) => s.demo);
  const savedIp = useStore((s) => s.settings.cameraIp);
  const [ip, setIp] = useState(savedIp);
  const [focus, setFocus] = useState(false);

  const valid = IP_RE.test(ip.trim());

  return (
    <div className="connect scroll">
      <div className="wordmark">
        <div className="logo">
          <ApertureIcon size={30} className="" />
        </div>
        <h1>
          Aero<span className="amber-text">Shutter</span>
        </h1>
        <span className="tag">Wi-Fi Film Loader</span>
      </div>

      <div className="developing">
        <div className="ticks" />
        <div className="ring" />
        <div className="ring" />
        <div className="ring" />
        <div className="sweep" />
        <div className={`core ${connecting ? 'on' : ''}`} />
      </div>
      <p className="searching-label">
        {connecting ? 'Developing connection…' : 'Standing by for your Nikon'}
      </p>

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
            <strong>Load the roll</strong>
            <span>AeroShutter finds the camera at 192.168.1.1 — or enter it below.</span>
          </div>
        </div>
      </div>

      <div className="ip-field">
        <div className="ip-label">Camera Address</div>
        <div className={`ip-wrap ${focus ? 'focus' : ''} ${ip && !valid ? 'invalid' : ''}`}>
          <span className="lead">IP</span>
          <input
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
      </div>

      <div className="connect-actions">
        <button
          className="btn btn-primary btn-block"
          disabled={connecting || !valid}
          onClick={() => void connect(ip.trim())}
        >
          {connecting ? 'Developing…' : 'Connect to camera'}
        </button>
        {demo && (
          <button className="btn btn-ghost btn-block" onClick={() => void enterDemo()}>
            Try demo mode
          </button>
        )}
        {connectError && <p className="error-line">{connectError}</p>}
      </div>

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
