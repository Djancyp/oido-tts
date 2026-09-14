import {useState} from 'react';
import {AudioLines, Podcast} from 'lucide-react';
import {ComposeView} from '@/components/ComposeView';
import {PodcastView} from '@/components/PodcastView';
import {Tooltip, TooltipContent, TooltipProvider, TooltipTrigger} from '@/components/ui/tooltip';
import {glassPanel} from '@/lib/ui';

type Tab = 'compose' | 'podcast';

const TABS: {id: Tab; label: string; icon: typeof AudioLines}[] = [
    {id: 'compose', label: 'Compose', icon: AudioLines},
    {id: 'podcast', label: 'Podcast', icon: Podcast},
];

function App() {
    const [tab, setTab] = useState<Tab>('compose');

    return (
        <TooltipProvider>
            <div
                className="relative flex h-screen overflow-hidden bg-[#f5f5f7]"
                style={{WebkitAppRegion: 'drag'} as React.CSSProperties}
            >
                {/* Ambient blurred color field the glass panels sit on top of */}
                <div className="pointer-events-none absolute inset-0 overflow-hidden">
                    <div className="absolute -left-20 -top-24 size-[420px] rounded-full bg-[#0066cc]/70 blur-3xl" />
                    <div className="absolute -right-16 top-1/4 size-[360px] rounded-full bg-[#7c5cff]/60 blur-3xl" />
                    <div className="absolute bottom-[-140px] left-1/3 size-[400px] rounded-full bg-[#2997ff]/55 blur-3xl" />
                </div>

                {/* pl leaves room for the icon rail (w-14, left-4, plus a gap) so
                    content never sits underneath it. no-drag opts this whole pane
                    back out of the outer div's drag region — without it, every
                    Textarea/range input/button in Compose and Podcast would sit
                    under an OS-level window-drag area that swallows mousedown
                    before it reaches the page, breaking text selection and the
                    playback scrubber. Both views stay mounted so switching tabs
                    mid-generation doesn't drop that job's progress/stream
                    listeners — only one can actually be generating at a time (see
                    App.beginJob on the backend), but its UI should keep working if
                    you tab away and back. */}
                <div
                    className="flex flex-1 overflow-hidden pl-[5.5rem]"
                    style={{WebkitAppRegion: 'no-drag'} as React.CSSProperties}
                >
                    <div className={tab === 'compose' ? 'contents' : 'hidden'}>
                        <ComposeView />
                    </div>
                    <div className={tab === 'podcast' ? 'contents' : 'hidden'}>
                        <PodcastView />
                    </div>
                </div>

                {/* Collapsed icon rail: switches Compose/Podcast, replaces the old
                    top title bar. Kept on the drag-region base div so the window is
                    still draggable from empty space around the rail; each button
                    opts back out of drag so clicks register. */}
                <nav
                    className={glassPanel(
                        'absolute left-4 top-4 bottom-4 z-10 flex w-14 shrink-0 flex-col items-center gap-1.5 py-4',
                    )}
                    style={{WebkitAppRegion: 'no-drag'} as React.CSSProperties}
                    aria-label="Views"
                >
                    {TABS.map(({id, label, icon: Icon}) => (
                        <Tooltip key={id}>
                            <TooltipTrigger asChild>
                                <button
                                    onClick={() => setTab(id)}
                                    aria-label={label}
                                    aria-current={tab === id ? 'page' : undefined}
                                    className={`flex size-10 items-center justify-center rounded-xl transition-colors ${
                                        tab === id
                                            ? 'bg-[#0066cc] text-white shadow-[0_2px_10px_rgba(0,102,204,0.35)]'
                                            : 'text-foreground/50 hover:bg-black/5 hover:text-foreground/80'
                                    }`}
                                >
                                    <Icon className="size-5" />
                                </button>
                            </TooltipTrigger>
                            <TooltipContent side="right">{label}</TooltipContent>
                        </Tooltip>
                    ))}
                </nav>
            </div>
        </TooltipProvider>
    );
}

export default App;
