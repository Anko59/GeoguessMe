import { useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Home from './pages/home/Home';
import Login from './pages/auth/Login';
import Signup from './pages/auth/Signup';
import GroupsList from './pages/groups/GroupsList';
import GroupJoin from './pages/groups/GroupJoin';
import GroupView from './pages/groups/GroupView';
import ProtectedRoute from './components/navigation/ProtectedRoute';
import AuthProvider from './context/AuthProvider';
import ForgotPassword from './pages/auth/ForgotPassword';
import ResetPassword from './pages/auth/ResetPassword';
import VerifyEmail from './pages/auth/VerifyEmail';
import AccountSettings from './pages/account/AccountSettings';
import PwaOnboarding from './components/pwa/PwaOnboarding';
import { usePushBootstrap } from './push/usePushBootstrap';

function AppChrome() {
    usePushBootstrap();
    useEffect(() => {
        let cancelled = false;
        let idleHandle: number | null = null;
        let timeoutHandle: number | null = null;

        const clearSchedule = () => {
            if (idleHandle !== null && 'cancelIdleCallback' in window) {
                window.cancelIdleCallback(idleHandle);
                idleHandle = null;
            }
            if (timeoutHandle !== null) {
                window.clearTimeout(timeoutHandle);
                timeoutHandle = null;
            }
        };

        const preload = () => {
            idleHandle = null;
            timeoutHandle = null;
            if (cancelled || document.visibilityState !== 'visible') return;
            void import('./components/camera/lenses/faceTracker')
                .then(({ preloadFaceTracker }) => preloadFaceTracker())
                .catch(() => {
                    // Camera startup retries on demand and reports a user-facing error.
                });
        };

        const schedule = () => {
            clearSchedule();
            if (document.visibilityState !== 'visible') return;
            if ('requestIdleCallback' in window) {
                idleHandle = window.requestIdleCallback(preload, { timeout: 5000 });
            } else {
                timeoutHandle = setTimeout(preload, 3000);
            }
        };

        document.addEventListener('visibilitychange', schedule);
        schedule();
        return () => {
            cancelled = true;
            document.removeEventListener('visibilitychange', schedule);
            clearSchedule();
        };
    }, []);
    return (
        <div className="app-root">
            <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/login" element={<Login />} />
                <Route path="/signup" element={<Signup />} />
                <Route path="/forgot-password" element={<ForgotPassword />} />
                <Route path="/reset-password" element={<ResetPassword />} />
                <Route path="/verify-email" element={<VerifyEmail />} />
                <Route
                    path="/groups"
                    element={
                        <ProtectedRoute>
                            <GroupsList />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/join"
                    element={
                        <ProtectedRoute>
                            <GroupJoin />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/create"
                    element={
                        <ProtectedRoute>
                            <GroupJoin />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/group/:id"
                    element={
                        <ProtectedRoute>
                            <GroupView />
                        </ProtectedRoute>
                    }
                />
                <Route
                    path="/settings"
                    element={
                        <ProtectedRoute>
                            <AccountSettings />
                        </ProtectedRoute>
                    }
                />
            </Routes>
            <PwaOnboarding />
        </div>
    );
}

function App() {
    return (
        <Router>
            <AuthProvider>
                <AppChrome />
            </AuthProvider>
        </Router>
    );
}

export default App;
