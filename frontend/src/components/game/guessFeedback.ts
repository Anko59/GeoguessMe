export type GuessFeedbackTone = 'miss' | 'low' | 'steady' | 'strong' | 'excellent';

export interface GuessFeedback {
    label: string;
    subtitle: string;
    tone: GuessFeedbackTone;
}

interface GuessFeedbackTier extends GuessFeedback {
    minimumScore: number;
}

const feedbackTiers: GuessFeedbackTier[] = [
    { minimumScore: 0, label: 'Cartographic Catastrophe', subtitle: 'The map had other plans.', tone: 'miss' },
    { minimumScore: 500, label: 'Wayward Squire', subtitle: 'A brave guess, miles from the mark.', tone: 'low' },
    { minimumScore: 1500, label: 'Roaming Herald', subtitle: 'You found the right realm eventually.', tone: 'steady' },
    { minimumScore: 2500, label: 'Roadside Scout', subtitle: 'A solid read of the landscape.', tone: 'steady' },
    { minimumScore: 3500, label: 'Keen Pathfinder', subtitle: 'Sharp eyes and a steadier compass.', tone: 'strong' },
    {
        minimumScore: 4200,
        label: 'Royal Cartographer',
        subtitle: 'That was a noble bit of navigation.',
        tone: 'excellent',
    },
    { minimumScore: 4800, label: 'Masterstroke', subtitle: 'Nearly pin-perfect.', tone: 'excellent' },
];

export function feedbackForScore(score: number): GuessFeedback {
    const normalizedScore = Number.isFinite(score) ? Math.min(5000, Math.max(0, Math.round(score))) : 0;
    const tier = [...feedbackTiers].reverse().find(({ minimumScore }) => normalizedScore >= minimumScore);
    return tier ?? feedbackTiers[0];
}
