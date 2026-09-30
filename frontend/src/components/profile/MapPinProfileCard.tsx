import type { ProfileMapPin } from '../../types';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../utils/mapPins';
import './MapPinProfileCard.css';

interface MapPinProfileCardProps {
    username: string;
    pin?: ProfileMapPin;
    ownProfile: boolean;
}

export default function MapPinProfileCard({ username, pin, ownProfile }: MapPinProfileCardProps) {
    return (
        <section className="profile-map-pin" aria-labelledby="profile-map-pin-title">
            <div className="profile-map-pin__artwork">
                <img
                    src={pin?.image_url ?? DEFAULT_MAP_PIN_IMAGE_URL}
                    alt={pin ? `${username}'s ${pin.name} map pin` : ''}
                    aria-hidden={pin ? undefined : true}
                />
            </div>
            <div className="profile-map-pin__details">
                <p className="profile-eyebrow">Map pin</p>
                <h2 id="profile-map-pin-title">{pin?.name ?? 'Standard marker'}</h2>
                {pin ? (
                    <>
                        <p>
                            Unlocked by <strong>{pin.unlocked_by.name}</strong>
                        </p>
                        <p>{pin.unlocked_by.description}</p>
                    </>
                ) : (
                    <p>{ownProfile ? 'Choose an unlocked pin in Settings.' : 'No custom map pin selected.'}</p>
                )}
            </div>
        </section>
    );
}
