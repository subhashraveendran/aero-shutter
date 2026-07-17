import { useEffect, useRef, useState } from 'react';
import { useStore } from '../store';
import type { CameraProperty } from '../lib/camera';
import { PropCode } from '../lib/ptpip/constants';
import { tap, ImpactStyle } from '../lib/haptics';
import {
  AlertIcon,
  CameraIcon,
  MinusIcon,
  PlayIcon,
  PlusIcon,
  ShutterIcon,
  SlidersIcon,
  StopIcon,
  TimelapseIcon,
  VideoIcon,
} from '../components/icons';
import {
  formatDuration,
  isComplete,
  normalizePlan,
  plannedDurationSec,
  remainingSec,
  type TimelapsePlan,
} from '../components/timelapse';

type Mode = 'photo' | 'video' | 'timelapse';

function PropStepper({ prop }: { prop: CameraProperty }) {
  const changeProp = useStore((s) => s.changeProp);
  const [bump, setBump] = useState(false);

  useEffect(() => {
    setBump(true);
    const t = window.setTimeout(() => setBump(false), 240);
    return () => window.clearTimeout(t);
  }, [prop.value]);

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
          <MinusIcon size={18} />
        </button>
        <div style={{ flex: 1 }} />
        <button className="step-btn" disabled={!canNext} onClick={() => step(1)} aria-label="Raise">
          <PlusIcon size={18} />
        </button>
      </div>
    </div>
  );
}

/** A stepper for a plain numeric value (interval seconds / shot count). */
function NumberStepper({
  label,
  value,
  display,
  min,
  onStep,
}: {
  label: string;
  value: number;
  display: string;
  min: number;
  onStep: (dir: number) => void;
}) {
  return (
    <div className="card prop-card">
      <div className="label">{label}</div>
      <div className="val">{display}</div>
      <div className="stepper">
        <button
          className="step-btn"
          disabled={value <= min}
          onClick={() => onStep(-1)}
          aria-label={`Decrease ${label}`}
        >
          <MinusIcon size={18} />
        </button>
        <div style={{ flex: 1 }} />
        <button className="step-btn" onClick={() => onStep(1)} aria-label={`Increase ${label}`}>
          <PlusIcon size={18} />
        </button>
      </div>
    </div>
  );
}

const MODES: { id: Mode; label: string; icon: React.ReactNode }[] = [
  { id: 'photo', label: 'Photo', icon: <CameraIcon size={16} /> },
  { id: 'video', label: 'Video', icon: <VideoIcon size={16} /> },
  { id: 'timelapse', label: 'Timelapse', icon: <TimelapseIcon size={16} /> },
];

/** Nikon exposure-program values that represent a movie/live-view mode. */
const MOVIE_PROGRAM_VALUES = new Set([5, 6]);

export function ControlScreen() {
  const props = useStore((s) => s.props);
  const loading = useStore((s) => s.loadingProps);
  const loadProps = useStore((s) => s.loadProps);
  const capture = useStore((s) => s.capture);
  const changeProp = useStore((s) => s.changeProp);
  const cameraModel = useStore((s) => s.cameraModel);
  const demo = useStore((s) => s.demo);
  const toast = useStore((s) => s.toast);

  const [mode, setMode] = useState<Mode>('photo');

  // --- Timelapse (intervalometer) local state ---
  const [intervalSec, setIntervalSec] = useState(5);
  const [shots, setShots] = useState<number | null>(60); // null = until stopped
  const [running, setRunning] = useState(false);
  const [taken, setTaken] = useState(0);
  const [secToNext, setSecToNext] = useState(0);
  const [elapsed, setElapsed] = useState(0);

  const timer = useRef<number | null>(null); // fires each capture
  const ticker = useRef<number | null>(null); // 1s countdown/elapsed
  const takenRef = useRef(0);
  const planRef = useRef<TimelapsePlan>(normalizePlan(5, 60));

  const stopTimers = () => {
    if (timer.current) window.clearInterval(timer.current);
    if (ticker.current) window.clearInterval(ticker.current);
    timer.current = null;
    ticker.current = null;
  };

  const stopTimelapse = (announce = true) => {
    if (!running && !timer.current) return;
    stopTimers();
    setRunning(false);
    setSecToNext(0);
    if (announce) {
      void tap(ImpactStyle.Medium);
      toast(`Timelapse stopped — ${takenRef.current} frame${takenRef.current === 1 ? '' : 's'}`, 'info');
    }
  };

  const startTimelapse = () => {
    const plan = normalizePlan(intervalSec, shots);
    planRef.current = plan;
    takenRef.current = 0;
    setTaken(0);
    setElapsed(0);
    setSecToNext(plan.intervalSec);
    setRunning(true);
    void tap(ImpactStyle.Heavy);
    toast(
      plan.totalShots === null
        ? `Timelapse started — every ${plan.intervalSec}s`
        : `Timelapse — ${plan.totalShots} frames, every ${plan.intervalSec}s`,
      'success',
    );

    const fire = () => {
      void capture();
      takenRef.current += 1;
      setTaken(takenRef.current);
      if (isComplete(planRef.current, takenRef.current)) {
        stopTimers();
        setRunning(false);
        setSecToNext(0);
        void tap(ImpactStyle.Medium);
        toast(`Timelapse complete — ${takenRef.current} frames`, 'success');
        return;
      }
      setSecToNext(planRef.current.intervalSec);
    };

    // Fire the first frame immediately, then on the interval.
    fire();
    if (isComplete(planRef.current, takenRef.current)) return;
    timer.current = window.setInterval(fire, plan.intervalSec * 1000);
    ticker.current = window.setInterval(() => {
      setElapsed((e) => e + 1);
      setSecToNext((s) => (s > 0 ? s - 1 : 0));
    }, 1000);
  };

  // Load props once.
  useEffect(() => {
    if (props.length === 0) void loadProps();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Stop cleanly on unmount.
  useEffect(() => () => stopTimers(), []);

  // Stop when navigating away from the control screen or disconnecting.
  const screen = useStore((s) => s.screen);
  const connected = useStore((s) => s.connected);
  useEffect(() => {
    if ((screen !== 'control' || !connected) && (running || timer.current)) {
      stopTimelapse(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [screen, connected]);

  const noProps = !loading && props.length === 0;

  // --- Video-mode feasibility: is there a *settable* movie exposure program? ---
  const modeProp = props.find((p) => p.code === PropCode.ExposureProgramMode);
  const movieValue = modeProp?.options.find((v) => MOVIE_PROGRAM_VALUES.has(v));
  const videoSupported = Boolean(modeProp?.writable && movieValue !== undefined);
  // Purely visual toggle used only in demo mode so the segmented control feels alive.
  const [demoVideoOn, setDemoVideoOn] = useState(false);

  const selectMode = (m: Mode) => {
    void tap();
    if (m !== 'timelapse' && running) stopTimelapse(false);
    if (m === 'video') {
      if (videoSupported && modeProp && movieValue !== undefined) {
        void changeProp(modeProp, movieValue);
      } else if (demo) {
        setDemoVideoOn(true);
      }
    } else if (demoVideoOn) {
      setDemoVideoOn(false);
    }
    setMode(m);
  };

  const plan = normalizePlan(intervalSec, shots);
  const remaining = remainingSec(plan, taken, secToNext);
  const totalDur = plannedDurationSec(plan);

  return (
    <div className="screen">
      <div className="topbar">
        <h1>Exposure</h1>
        <div className="spacer" />
        <span className="readout">
          {running && (
            <span className="r live">
              <span className="rec-dot" /> REC
            </span>
          )}
          <span className="r">{cameraModel || 'CAMERA'}</span>
        </span>
      </div>

      <div className="mode-switch" role="tablist" aria-label="Capture mode">
        {MODES.map((m) => (
          <button
            key={m.id}
            role="tab"
            aria-selected={mode === m.id}
            className={`mode-seg ${mode === m.id ? 'active' : ''}`}
            onClick={() => selectMode(m.id)}
            disabled={running && m.id !== 'timelapse'}
          >
            {m.icon}
            {m.label}
          </button>
        ))}
      </div>

      <div className="scroll">
        {noProps && mode === 'photo' ? (
          <div className="control-empty">
            <span className="glyph" style={{ margin: '0 auto' }}>
              <SlidersIcon size={30} />
            </span>
            <strong style={{ fontFamily: 'var(--font-display)', fontSize: 19, color: 'var(--text)' }}>
              Controls unavailable
            </strong>
            <span style={{ maxWidth: 280 }}>
              This camera doesn’t expose writable settings over Wi-Fi. You can still browse and
              import frames.
            </span>
          </div>
        ) : mode === 'video' ? (
          <VideoPanel supported={videoSupported} demo={demo} demoOn={demoVideoOn} />
        ) : mode === 'timelapse' ? (
          <div className="control-list stagger">
            <NumberStepper
              label="Interval"
              value={intervalSec}
              display={`${intervalSec}s`}
              min={1}
              onStep={(d) => {
                if (running) return;
                setIntervalSec((v) => Math.max(1, v + d * (v >= 30 ? 5 : v >= 10 ? 2 : 1)));
              }}
            />
            <NumberStepper
              label="Frames"
              value={shots ?? 0}
              display={shots === null ? '∞' : String(shots)}
              min={0}
              onStep={(d) => {
                if (running) return;
                setShots((v) => {
                  if (v === null) return d < 0 ? 120 : null;
                  const next = v + d * (v >= 100 ? 20 : v >= 20 ? 10 : 5);
                  return next < 5 ? null : next; // step below floor -> "until stopped"
                });
              }}
            />
            <div className="card prop-card full tl-plan">
              <div className="label">Sequence</div>
              <div className="tl-plan-body">
                <span>
                  {shots === null
                    ? 'Runs until you stop'
                    : `${shots} frames over ${formatDuration(totalDur ?? 0)}`}
                </span>
                <span className="tl-sub">One shutter release every {plan.intervalSec}s</span>
              </div>
            </div>
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

      {/* Shutter dock: Photo = shutter release; Timelapse = start/stop */}
      {mode === 'photo' && !noProps && (
        <div className="shutter-dock">
          <button
            className="shutter"
            aria-label="Release shutter"
            onClick={() => {
              void tap(ImpactStyle.Heavy);
              void capture();
            }}
          >
            <div className="inner">
              <ShutterIcon size={30} className="shutter-glyph" />
            </div>
          </button>
          <span className="shutter-label">Release</span>
        </div>
      )}

      {mode === 'timelapse' && (
        <div className="shutter-dock timelapse-dock">
          {running && (
            <div className="tl-progress" aria-live="polite">
              <span className="tl-count">
                {String(taken).padStart(2, '0')}
                {shots !== null && ` / ${shots}`}
              </span>
              <span className="tl-readout">
                {shots !== null && isComplete(plan, taken)
                  ? 'complete'
                  : `next in ${secToNext}s · elapsed ${formatDuration(elapsed)}${
                      remaining !== null ? ` · left ${formatDuration(remaining)}` : ''
                    }`}
              </span>
            </div>
          )}
          <button
            className={`shutter tl-shutter ${running ? 'running' : ''}`}
            aria-label={running ? 'Stop timelapse' : 'Start timelapse'}
            onClick={() => (running ? stopTimelapse() : startTimelapse())}
          >
            <div className="inner">
              {running ? (
                <StopIcon size={30} className="shutter-glyph" />
              ) : (
                <PlayIcon size={30} className="shutter-glyph" />
              )}
            </div>
          </button>
          <span className="shutter-label">{running ? 'Stop' : 'Start timelapse'}</span>
        </div>
      )}
    </div>
  );
}

function VideoPanel({
  supported,
  demo,
  demoOn,
}: {
  supported: boolean;
  demo: boolean;
  demoOn: boolean;
}) {
  if (supported) {
    return (
      <div className="control-list stagger">
        <div className="card prop-card full video-live">
          <div className="label">Live view</div>
          <div className="tl-plan-body">
            <span>Camera switched to movie mode.</span>
            <span className="tl-sub">Recording is controlled from the camera body.</span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="control-empty video-unavailable">
      <span className={`glyph ${demo && demoOn ? 'demo-on' : 'muted'}`} style={{ margin: '0 auto' }}>
        <VideoIcon size={30} />
      </span>
      <strong style={{ fontFamily: 'var(--font-display)', fontSize: 19, color: 'var(--text)' }}>
        {demo && demoOn ? 'Video mode (simulated)' : 'Video isn’t available over Wi-Fi'}
      </strong>
      <span style={{ maxWidth: 300 }}>
        {demo && demoOn
          ? 'This is a visual preview only. On a real D5300, the Wi-Fi link can’t switch the camera into movie mode or start recording — that stays on the camera body.'
          : 'This camera doesn’t expose a settable movie mode over its Wi-Fi link, so AeroShutter can’t start or record video. Use Photo or Timelapse instead.'}
      </span>
      <div className="video-note">
        <AlertIcon size={16} />
        <span>Recording is never faked — the shutter controls are disabled here.</span>
      </div>
    </div>
  );
}
