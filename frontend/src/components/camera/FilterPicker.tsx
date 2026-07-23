import type { CSSProperties } from 'react';
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

    return (
        <div className="camera-filter-picker" role="group" aria-label="Photo filters">
            <div className="camera-filter-heading">
                <span className="camera-filter-label">Lenses</span>
                <span className="camera-filter-current">{selectedFilterOption?.label}</span>
            </div>
            <div className="camera-filter-options">
                {LENS_OPTIONS.map((option) => (
                    <button
                        key={option.id}
                        type="button"
                        className={`camera-filter-option ${selectedFilter === option.id ? 'selected' : ''}`}
                        style={{ '--lens-accent': option.accent } as CSSProperties}
                        aria-pressed={selectedFilter === option.id}
                        aria-label={option.label}
                        title={option.label}
                        onClick={() => onSelect(option.id)}
                    >
                        <span className="camera-filter-icon" aria-hidden="true">
                            {option.icon}
                        </span>
                        <span className="camera-filter-option-label">{option.label}</span>
                    </button>
                ))}
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
