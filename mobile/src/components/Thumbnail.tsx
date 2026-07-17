import { useEffect, useRef, useState } from 'react';
import { camera, type Photo } from '../lib/camera';
import { CheckIcon } from './icons';

const cache = new Map<number, string>();

interface ThumbnailProps {
  photo: Photo;
  imported: boolean;
  selected: boolean;
  frameNo: number;
  onClick: () => void;
  onLongPress: () => void;
}

export function Thumbnail({
  photo,
  imported,
  selected,
  frameNo,
  onClick,
  onLongPress,
}: ThumbnailProps) {
  const [src, setSrc] = useState<string | null>(cache.get(photo.handle) ?? null);
  const ref = useRef<HTMLDivElement>(null);
  const pressTimer = useRef<number | null>(null);
  const longFired = useRef(false);

  useEffect(() => {
    if (src) return;
    let cancelled = false;
    const el = ref.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          observer.disconnect();
          camera
            .thumbnail(photo.handle)
            .then((url) => {
              if (cancelled) return;
              cache.set(photo.handle, url);
              setSrc(url);
            })
            .catch(() => undefined);
        }
      },
      { rootMargin: '200px' },
    );
    observer.observe(el);
    return () => {
      cancelled = true;
      observer.disconnect();
    };
  }, [photo.handle, src]);

  const startPress = () => {
    longFired.current = false;
    pressTimer.current = window.setTimeout(() => {
      longFired.current = true;
      onLongPress();
    }, 380);
  };
  const endPress = () => {
    if (pressTimer.current) window.clearTimeout(pressTimer.current);
  };

  return (
    <div
      ref={ref}
      className={`tile ${selected ? 'selected' : ''}`}
      onPointerDown={startPress}
      onPointerUp={endPress}
      onPointerLeave={endPress}
      onClick={() => {
        if (!longFired.current) onClick();
      }}
      role="button"
      aria-label={photo.filename}
    >
      {src ? (
        <img src={src} alt={photo.filename} loading="lazy" />
      ) : (
        <div className="skeleton" style={{ width: '100%', height: '100%' }} />
      )}
      {photo.format === 'RAW' && <span className="fmt raw">RAW</span>}
      {photo.format === 'JPEG' && <span className="fmt">JPG</span>}
      {photo.format === 'OTHER' && <span className="fmt">FILE</span>}
      <span className="frame-no">{String(frameNo).padStart(2, '0')}</span>
      {imported && !selected && <span className="imported-dot" />}
      {selected && (
        <span className="check">
          <CheckIcon size={14} className="" />
        </span>
      )}
    </div>
  );
}
