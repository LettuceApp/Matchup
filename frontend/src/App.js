import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import HomePage from './pages/HomePage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';  // Correctly Imported
import MatchupPage from './pages/MatchupPage';    // Correctly Imported
import CreateMatchup from './pages/CreateMatchup'; // Import CreateMatchup page
import UserProfile from './pages/UserProfile';

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="users/:uid/matchup/:id" element={<MatchupPage />} />
        <Route path="/users/:userId/create-matchup" element={<CreateMatchup />} />
        <Route path="/users/:userId/profile" element={<UserProfile />} />
      </Routes>
    </Router>
  );
}

export default App;
