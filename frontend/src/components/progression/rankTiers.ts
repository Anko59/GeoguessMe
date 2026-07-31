export const TIER_COUNT = 5;

/** Tier of a progression rank level. Badge artwork is shared within a tier so
 *  the material step-up (bronze → silver → gold → crown → imperial crown) is
 *  readable at a glance, while the rank name disambiguates within a tier. */
export function tierForLevel(level: number): number {
    if (level >= 17) return 5;
    if (level >= 13) return 4;
    if (level >= 9) return 3;
    if (level >= 5) return 2;
    return 1;
}
