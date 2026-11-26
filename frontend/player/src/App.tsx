import { useAuth } from "./lib/auth";
import AuthPage from "./pages/Auth";
import GamePage from "./pages/Game";

// Two screens only, switched on the session: no router needed.
export default function App() {
  const { user } = useAuth();
  return user ? <GamePage /> : <AuthPage />;
}
