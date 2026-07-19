import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';

interface ProtectedRouteProps {
    children: React.ReactNode;
}

export default function ProtectedRoute({ children }: ProtectedRouteProps) {
    const { loading, isAuthenticated } = useAuth();
    const location = useLocation();
    if (loading) return <div className="loading">Restoring session…</div>;
    if (!isAuthenticated) {
        const from = `${location.pathname}${location.search}${location.hash}`;
        return <Navigate to="/login" replace state={{ from }} />;
    }
    return <>{children}</>;
}
