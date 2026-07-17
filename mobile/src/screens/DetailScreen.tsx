import { useEffect, useState } from 'react';
import { Share } from '@capacitor/share';
import { useStore } from '../store';
import { camera } from '../lib/camera';
import { identityKey } from '../lib/db';
import { formatBytes, formatDate } from '../lib/format';
import { ChevronLeft, DownloadIcon, ShareIcon } from '../components/icons';

export function DetailScreen() {
  const photos = useStore((s) => s.photos);
  const index = useStore((s) => s.detailIndex);
  const setIndex = useStore((s) => s.setDetailIndex);
  const navigate = useStore((s) => s.navigate);
  const importedIds = useStore((s) => s.importedIds);
  const importPhotos = useStore((s) => s.importPhotos);
  const toast = useStore((s) => s.toast);

  const photo = photos[index];
  const [fullSrc, setFullSrc] = useState<string | null>(null);
  const [thumbSrc, setThumbSrc] = useState<string | null>(null);
  const [touchStart, setTouchStart] = useState<number | null>(null);

  useEffect(() => {
    if (!photo) return;
    setFullSrc(null);
    let cancelled = false;
    camera
      .thumbnail(photo.handle)
      .then((s) => !cancelled && setThumbSrc(s))
      .catch(() => undefined);
    camera
      .fullImage(photo.handle)
      .then((s) => !cancelled && setFullSrc(s))
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [photo?.handle]);

  if (!photo) {
    navigate('gallery');
    return null;
  }

  const imported = importedIds.has(identityKey(photo.filename, photo.size));

  const go = (dir: number) => {
    const next = index + dir;
    if (next >= 0 && next < photos.length) setIndex(next);
  };

  const onShare = async () => {
    try {
      await Share.share({ title: photo.filename, text: `${photo.filename} from AeroShutter` });
    } catch {
      toast('Sharing is not available here', 'error');
    }
  };

  return (
    <div className="detail">
      {(fullSrc ?? thumbSrc) && (
        <div className="detail-ambient" style={{ backgroundImage: `url(${fullSrc ?? thumbSrc})` }} />
      )}

      <div className="detail-top">
        <button className="icon-btn" onClick={() => navigate('gallery')} aria-label="Back">
          <ChevronLeft size={24} className="" />
        </button>
        <span className="fname">{photo.filename}</span>
        <div style={{ flex: 1 }} />
        <span className="counter">
          {String(index + 1).padStart(2, '0')}/{String(photos.length).padStart(2, '0')}
        </span>
      </div>

      <div
        className="detail-stage"
        onTouchStart={(e) => setTouchStart(e.touches[0].clientX)}
        onTouchEnd={(e) => {
          if (touchStart === null) return;
          const dx = e.changedTouches[0].clientX - touchStart;
          if (Math.abs(dx) > 60) go(dx < 0 ? 1 : -1);
          setTouchStart(null);
        }}
      >
        {fullSrc || thumbSrc ? (
          <div className="film-frame">
            <img key={photo.handle} src={fullSrc ?? thumbSrc ?? ''} alt={photo.filename} />
          </div>
        ) : (
          <div className="skeleton" style={{ width: '80%', height: '60%', borderRadius: 4 }} />
        )}
      </div>

      <div className="detail-sheet">
        <div className="sheet-grip" />
        <div className="meta-title">Frame Data</div>
        <div className="meta-row">
          <span className="k">Name</span>
          <span className="v">{photo.filename}</span>
        </div>
        <div className="meta-row">
          <span className="k">Format</span>
          <span className="v">{photo.format}</span>
        </div>
        <div className="meta-row">
          <span className="k">Size</span>
          <span className="v">{formatBytes(photo.size)}</span>
        </div>
        <div className="meta-row">
          <span className="k">Dimensions</span>
          <span className="v">
            {photo.width} × {photo.height}
          </span>
        </div>
        <div className="meta-row">
          <span className="k">Captured</span>
          <span className="v">{formatDate(photo.captureEpochMs)}</span>
        </div>
        <div className="meta-row">
          <span className="k">Handle</span>
          <span className="v">0x{photo.handle.toString(16).toUpperCase()}</span>
        </div>
        <div className="meta-row">
          <span className="k">Imported</span>
          <span className="v" style={{ color: imported ? 'var(--success)' : 'var(--text-dim)' }}>
            {imported ? 'YES' : 'NO'}
          </span>
        </div>

        <div className="detail-actions">
          <button
            className="btn btn-primary"
            disabled={imported}
            onClick={() => void importPhotos([photo])}
          >
            <DownloadIcon size={18} className="" />
            {imported ? 'Imported' : 'Import'}
          </button>
          <button className="btn btn-ghost" onClick={() => void onShare()}>
            <ShareIcon size={18} className="" />
            Share
          </button>
        </div>
      </div>
    </div>
  );
}
