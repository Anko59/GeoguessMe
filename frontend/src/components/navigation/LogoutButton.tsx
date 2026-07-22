import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import Icon from '../ui/Icon';
import './LogoutButton.css';

export default function LogoutButton() {
    const navigate = useNavigate();
    const { logout } = useAuth();

    const handleLogout = async (): Promise<void> => {
        // Leave the protected route before auth state is cleared. Otherwise
        // ProtectedRoute can redirect to /login while the logout request is
        // settling, racing the intended public landing page navigation.
        navigate('/', { replace: true, state: { loggingOut: true } });
        try {
            await logout();
        } catch {
            // AuthProvider clears local credentials in its finally block;
            // local sign-out is complete even if server revocation fails.
        }
    };

    return (
        <button className="logout-btn" onClick={handleLogout}>
            <Icon name="logout" className="logout-icon" />
            Logout
        </button>
    );
}
