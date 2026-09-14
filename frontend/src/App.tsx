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
                className="relative flex h-screen overflow-hidden bg-background"
                style={{WebkitAppRegion: 'drag'} as React.CSSProperties}
            >
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
                                    className={`flex size-10 items-center justify-center rounded-lg border-l-2 transition-colors ${
                                        tab === id
                                            ? 'border-primary bg-accent text-foreground'
                                            : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground'
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
