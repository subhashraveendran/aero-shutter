import { useMemo, useRef, useState } from 'react';
import { useStore, type FilterChip } from '../store';
import { identityKey } from '../lib/db';
import type { Photo } from '../lib/camera';
import { Thumbnail } from '../components/Thumbnail';
import { BatteryIcon, CameraIcon, DownloadIcon, WifiIcon, XIcon } from '../components/icons';

const CHIPS: { id: FilterChip; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'new', label: 'New' },
  { id: 'raw', label: 'RAW' },
  { id: 'jpeg', label: 'JPEG' },
  { id: 'imported', label: 'Imported' },
];

function matches(photo: Photo, filter: FilterChip, imported: boolean): boolean {
  switch (filter) {
    case 'new':
      return !imported;
    case 'raw':
      return photo.format === 'RAW';
    case 'jpeg':
      return photo.format === 'JPEG';
    case 'imported':
      return imported;
    default:
      return true;
  }
}

export function GalleryScreen() {
  const photos = useStore((s) => s.photos);
  const loading = useStore((s) => s.loadingPhotos);
  const importedIds = useStore((s) => s.importedIds);
  const filter = useStore((s) => s.filter);
  const setFilter = useStore((s) => s.setFilter);
  const selection = useStore((s) => s.selection);
  const toggleSelect = useStore((s) => s.toggleSelect);
  const clearSelection = useStore((s) => s.clearSelection);
  const openDetail = useStore((s) => s.openDetail);
  const refreshPhotos = useStore((s) => s.refreshPhotos);
  const importNew = useStore((s) => s.importNew);
  const importPhotos = useStore((s) => s.importPhotos);
  const cameraModel = useStore((s) => s.cameraModel);

  const scrollRef = useRef<HTMLDivElement>(null);
  const pullStart = useRef(0);
  const [pull, setPull] = useState(0);

  const counts = useMemo(() => {
    const isNew = (p: Photo) => !importedIds.has(identityKey(p.filename, p.size));
    return {
      all: photos.length,
      new: photos.filter(isNew).length,
      raw: photos.filter((p) => p.format === 'RAW').length,
      jpeg: photos.filter((p) => p.format === 'JPEG').length,
      imported: photos.filter((p) => !isNew(p)).length,
    } as Record<FilterChip, number>;
  }, [photos, importedIds]);

  const visible = useMemo(() => {
    return photos.filter((p) =>
      matches(p, filter, importedIds.has(identityKey(p.filename, p.size))),
    );
  }, [photos, filter, importedIds]);

  const newCount = counts.new;
  const selectionCount = selection.size;

  const onTouchStart = (e: React.TouchEvent) => {
    if (scrollRef.current && scrollRef.current.scrollTop <= 0) {
      pullStart.current = e.touches[0].clientY;
    } else {
      pullStart.current = -1;
    }
  };
  const onTouchMove = (e: React.TouchEvent) => {
    if (pullStart.current < 0) return;
    const delta = e.touches[0].clientY - pullStart.current;
    if (delta > 0) setPull(Math.min(delta * 0.4, 70));
  };
  const onTouchEnd = () => {
    if (pull > 50) void refreshPhotos();
    setPull(0);
    pullStart.current = -1;
  };

  const doImportSelection = () => {
    const chosen = photos.filter((p) => selection.has(p.handle));
    void importPhotos(chosen);
  };

  return (
    <div className="screen">
      <div className="topbar">
        <h1>{selectionCount > 0 ? 'Select' : 'Contact Sheet'}</h1>
        <div className="spacer" />
        {selectionCount > 0 ? (
          <button className="badge badge-demo" onClick={clearSelection}>
            <XIcon size={12} className="" /> Cancel
          </button>
        ) : (
          <div className="readout">
            <span className="r live">
              <WifiIcon size={13} className="" />
              {cameraModel ? cameraModel.split(' ').slice(-1)[0] : 'LINK'}
            </span>
            <span className="r">
              <BatteryIcon size={15} className="" />
              84%
            </span>
            <span className="r">{String(photos.length).padStart(3, '0')} FRM</span>
          </div>
        )}
      </div>

      <div className="chips">
        {CHIPS.map((c) => (
          <button
            key={c.id}
            className={`chip ${filter === c.id ? 'active' : ''}`}
            onClick={() => setFilter(c.id)}
          >
            {c.label}
            <span className="count">{counts[c.id]}</span>
          </button>
        ))}
      </div>

      <div
        className="scroll"
        ref={scrollRef}
        onTouchStart={onTouchStart}
        onTouchMove={onTouchMove}
        onTouchEnd={onTouchEnd}
      >
        {pull > 0 && (
          <div className="pull-hint" style={{ height: pull }}>
            {pull > 50 ? 'Release to develop' : 'Pull to refresh'}
          </div>
        )}

        {loading && photos.length === 0 ? (
          <div className="contact-sheet">
            <div className="grid">
              {Array.from({ length: 12 }).map((_, i) => (
                <div key={i} className="tile skeleton" />
              ))}
            </div>
          </div>
        ) : visible.length === 0 ? (
          <div className="empty">
            <span className="glyph">
              <CameraIcon size={30} className="" />
            </span>
            <strong>No frames here</strong>
            <span>
              {filter === 'all'
                ? 'Take some shots on the camera, then pull to develop.'
                : 'Nothing matches this filter.'}
            </span>
          </div>
        ) : (
          <div className="contact-sheet">
            <div className="grid">
              {visible.map((photo, i) => (
                <Thumbnail
                  key={photo.handle}
                  photo={photo}
                  frameNo={i + 1}
                  imported={importedIds.has(identityKey(photo.filename, photo.size))}
                  selected={selection.has(photo.handle)}
                  onClick={() =>
                    selectionCount > 0
                      ? toggleSelect(photo.handle)
                      : openDetail(photos.indexOf(photo))
                  }
                  onLongPress={() => toggleSelect(photo.handle)}
                />
              ))}
            </div>
          </div>
        )}
      </div>

      {selectionCount > 0 ? (
        <div className="selbar">
          <span className="sel-count">{String(selectionCount).padStart(2, '0')} selected</span>
          <button className="fab inline" onClick={doImportSelection}>
            <DownloadIcon size={18} className="" />
            Import <span className="fab-count">{selectionCount}</span>
          </button>
        </div>
      ) : (
        newCount > 0 && (
          <button className="fab" onClick={() => void importNew()}>
            <DownloadIcon size={18} className="" />
            Import <span className="fab-count">{newCount}</span> new
          </button>
        )
      )}
    </div>
  );
}
