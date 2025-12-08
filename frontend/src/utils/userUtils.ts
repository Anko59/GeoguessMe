/**
 * Utility functions for user management
 */

/**
 * Gets the current user ID from localStorage
 * @returns The user ID or empty string if not found
 */
export const getCurrentUserId = (): string => {
    try {
        const user = JSON.parse(localStorage.getItem('user') || '{}');
        return user.id || '';
    } catch (error) {
        console.error('Error getting current user ID:', error);
        return '';
    }
};
