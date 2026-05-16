import { CameraView } from './components/CameraView';
import { PassphraseGate } from './components/PassphraseGate';

export function App() {
  return (
    <main className="w-full h-screen bg-black">
      <PassphraseGate>
        <CameraView />
      </PassphraseGate>
    </main>
  );
}
