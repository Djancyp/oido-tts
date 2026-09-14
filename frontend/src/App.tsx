import {useState} from 'react';
import {ComposeView} from '@/components/ComposeView';
import {PodcastView} from '@/components/PodcastView';

type Tab = 'compose' | 'podcast';

function App() {
    const [tab, setTab] = useState<Tab>('compose');

    return (
        <div className="relative flex h-screen flex-col overflow-hidden bg-[#f5f5f7]">
            {/* Ambient blurred color field the glass panels sit on top of */}
            <div className="pointer-events-none absolute inset-0 overflow-hidden">
                <div className="absolute -left-20 -top-24 size-[420px] rounded-full bg-[#0066cc]/70 blur-3xl" />
                <div className="absolute -right-16 top-1/4 size-[360px] rounded-full bg-[#7c5cff]/60 blur-3xl" />
                <div className="absolute bottom-[-140px] left-1/3 size-[400px] rounded-full bg-[#2997ff]/55 blur-3xl" />
            </div>

            {/* Title bar */}
            <div
                className="flex h-11 shrink-0 items-center gap-4 border-b border-white/10 bg-black/70 px-4 text-xs font-medium text-white backdrop-blur-md"
                style={{WebkitAppRegion: 'drag'} as React.CSSProperties}
            >
                <span>oido-tts</span>
                <nav className="flex items-center gap-1" style={{WebkitAppRegion: 'no-drag'} as React.CSSProperties}>
                    <button
                        onClick={() => setTab('compose')}
                        className={`rounded-full px-3 py-1 transition-colors ${tab === 'compose' ? 'bg-white/20 text-white' : 'text-white/60 hover:text-white/90'}`}
                    >
                        Compose
                    </button>
                    <button
                        onClick={() => setTab('podcast')}
                        className={`rounded-full px-3 py-1 transition-colors ${tab === 'podcast' ? 'bg-white/20 text-white' : 'text-white/60 hover:text-white/90'}`}
                    >
                        Podcast
                    </button>
                </nav>
            </div>

            {/* Both stay mounted so switching tabs mid-generation doesn't drop
                that job's progress/stream listeners — only one can actually be
                generating at a time (see App.beginJob on the backend), but its
                UI should keep working if you tab away and back. */}
            <div className={tab === 'compose' ? 'contents' : 'hidden'}>
                <ComposeView />
            </div>
            <div className={tab === 'podcast' ? 'contents' : 'hidden'}>
                <PodcastView />
            </div>
        </div>
    );
}

export default App;
