import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Home from './pages/Home';
import Login from './pages/Login';
import Signup from './pages/Signup';
import GroupsList from './pages/GroupsList';
import GroupJoin from './pages/GroupJoin';
import GroupView from './pages/GroupView';
import ProtectedRoute from './components/ProtectedRoute';

function App() {
  return (
    <Router>
      <div className="container">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/login" element={<Login />} />
          <Route path="/signup" element={<Signup />} />
          <Route path="/groups" element={<ProtectedRoute><GroupsList /></ProtectedRoute>} />
          <Route path="/group/join" element={<ProtectedRoute><GroupJoin /></ProtectedRoute>} />
          <Route path="/group/create" element={<ProtectedRoute><GroupJoin /></ProtectedRoute>} />
          <Route path="/group/:id" element={<ProtectedRoute><GroupView /></ProtectedRoute>} />
        </Routes>
      </div>
    </Router>
  );
}

export default App;
