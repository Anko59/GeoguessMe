import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import './LogoutButton.css';

export default function LogoutButton() {
    const navigate = useNavigate();
    const { logout } = useAuth();

    const handleLogout = async (): Promise<void> => {
        // Leaving the protected page must not depend on the revocation
        // request succeeding. AuthProvider clears local credentials in its
        // own finally block; always navigate away even when the server or
        // network is unavailable.
        try {
            await logout();
        } catch {
            // A local sign-out is still complete when the revocation request
            // fails; do not leave an unhandled rejection on the page.
        } finally {
            navigate('/');
        }
    };

    return (
        <button className="logout-btn" onClick={handleLogout}>
            <img src="/logout_icon.png" alt="Logout" className="logout-icon" />
            Logout
        </button>
    );
}
