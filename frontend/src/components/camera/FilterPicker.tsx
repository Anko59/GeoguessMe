import { FACE_FILTER_OPTIONS, type FaceFilterId } from '../../faceFilters';

interface FilterPickerProps {
    selectedFilter: FaceFilterId;
    filterReady: boolean;
    filterError: string;
    onSelect: (filter: FaceFilterId) => void;
}

export default function FilterPicker({ selectedFilter, filterReady, filterError, onSelect }: FilterPickerProps) {
    const selectedFilterOption = FACE_FILTER_OPTIONS.find((option) => option.id === selectedFilter);

    return (
        <div className="camera-filter-picker" role="group" aria-label="Photo filters">
            <div className="camera-filter-heading">
                <span className="camera-filter-label">Try a lens</span>
                <span className="camera-filter-current">{selectedFilterOption?.label}</span>
            </div>
            <div className="camera-filter-options">
                {FACE_FILTER_OPTIONS.map((option) => (
                    <button
                        key={option.id}
                        type="button"
                        className={`camera-filter-option ${selectedFilter === option.id ? 'selected' : ''}`}
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
            {selectedFilter !== 'none' && !filterReady && !filterError && <small>Loading face tracking…</small>}
            {filterError && <small className="camera-filter-status">{filterError}</small>}
        </div>
    );
}
