import type { GroupChallenge } from '../../types';

export function locationLabel(item: GroupChallenge) {
    if (item.lat !== undefined && item.long !== undefined) return `${item.lat.toFixed(2)}°, ${item.long.toFixed(2)}°`;
    if (item.location_reveals_at) return `Location hidden until ${new Date(item.location_reveals_at).toLocaleString()}`;
    return 'Guess this challenge to reveal its location';
}

export function challengeStatusLabel(item: GroupChallenge) {
    return { available: 'Ready to play', results: 'Your challenge', guessed: 'Played', expired: 'Ended' }[item.status];
}
