import { useRef, type CSSProperties, type PointerEvent, type WheelEvent } from 'react';
import { LENS_OPTIONS, type LensId } from './lenses/lensCatalog';

interface FilterPickerProps {
    selectedFilter: LensId;
    filterReady: boolean;
    filterError: string;
    faceDetected: boolean;
    onSelect: (filter: LensId) => void;
}

export default function FilterPicker({
    selectedFilter,
    filterReady,
    filterError,
    faceDetected,
    onSelect,
}: FilterPickerProps) {
    const selectedFilterOption = LENS_OPTIONS.find((option) => option.id === selectedFilter);
    const optionsRef = useRef<HTMLDivElement>(null);
    const dragRef = useRef({ pointerId: -1, startX: 0, scrollLeft: 0, dragging: false });
    const suppressClickRef = useRef(false);

    const scrollOptions = (direction: -1 | 1) => {
        const options = optionsRef.current;
        if (!options) return;
        options.scrollBy({ left: direction * Math.max(320, options.clientWidth * 0.72), behavior: 'smooth' });
    };

    const startDrag = (event: PointerEvent<HTMLDivElement>) => {
        const options = optionsRef.current;
        if (!options || event.pointerType === 'touch') return;
        dragRef.current = {
            pointerId: event.pointerId,
            startX: event.clientX,
            scrollLeft: options.scrollLeft,
            dragging: false,
        };
        suppressClickRef.current = false;
    };

    const drag = (event: PointerEvent<HTMLDivElement>) => {
        const options = optionsRef.current;
        if (!options || dragRef.current.pointerId !== event.pointerId) return;
        const distance = event.clientX - dragRef.current.startX;
        if (Math.abs(distance) <= 5) return;
        if (!dragRef.current.dragging) {
            dragRef.current.dragging = true;
            suppressClickRef.current = true;
            options.setPointerCapture(event.pointerId);
            options.classList.add('dragging');
        }
        options.scrollLeft = dragRef.current.scrollLeft - distance;
    };

    const finishDrag = (event: PointerEvent<HTMLDivElement>) => {
        const options = optionsRef.current;
        if (!options || dragRef.current.pointerId !== event.pointerId) return;
        if (options.hasPointerCapture(event.pointerId)) options.releasePointerCapture(event.pointerId);
        options.classList.remove('dragging');
        dragRef.current.pointerId = -1;
        dragRef.current.dragging = false;
    };

    const wheel = (event: WheelEvent<HTMLDivElement>) => {
        const options = optionsRef.current;
        if (!options || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return;
        event.preventDefault();
        options.scrollLeft += event.deltaY;
    };

    return (
        <div className="camera-filter-picker" role="group" aria-label="Photo filters">
            <div className="camera-filter-heading">
                <span className="camera-filter-label">Lenses</span>
                <span className="camera-filter-current">{selectedFilterOption?.label}</span>
            </div>
            <div className="camera-filter-rail">
                <button
                    type="button"
                    className="camera-filter-arrow previous"
                    aria-label="Previous lenses"
                    onClick={() => scrollOptions(-1)}
                >
                    ‹
                </button>
                <div
                    ref={optionsRef}
                    className="camera-filter-options"
                    onPointerDown={startDrag}
                    onPointerMove={drag}
                    onPointerUp={finishDrag}
                    onPointerCancel={finishDrag}
                    onWheel={wheel}
                >
                    {LENS_OPTIONS.map((option) => (
                        <button
                            key={option.id}
                            type="button"
                            className={`camera-filter-option ${selectedFilter === option.id ? 'selected' : ''}`}
                            style={
                                {
                                    '--lens-accent': option.accent,
                                    '--lens-preview': option.preview ? `url("${option.preview}")` : 'none',
                                } as CSSProperties
                            }
                            aria-pressed={selectedFilter === option.id}
                            aria-label={option.label}
                            title={option.label}
                            onClick={() => {
                                if (suppressClickRef.current) {
                                    suppressClickRef.current = false;
                                    return;
                                }
                                onSelect(option.id);
                            }}
                        >
                            <span
                                className={`camera-filter-icon ${option.preview ? 'has-preview' : ''}`}
                                aria-hidden="true"
                            >
                                {option.preview ? null : option.icon}
                            </span>
                            <span className="camera-filter-option-label">{option.label}</span>
                        </button>
                    ))}
                </div>
                <button
                    type="button"
                    className="camera-filter-arrow next"
                    aria-label="Next lenses"
                    onClick={() => scrollOptions(1)}
                >
                    ›
                </button>
            </div>
            {selectedFilter !== 'none' && !filterReady && !filterError && (
                <small className="camera-filter-status">Loading 3D face tracking…</small>
            )}
            {selectedFilter !== 'none' && filterReady && !faceDetected && !filterError && (
                <small className="camera-filter-status">Center your face in good light</small>
            )}
            {filterError && <small className="camera-filter-status">{filterError}</small>}
        </div>
    );
}
