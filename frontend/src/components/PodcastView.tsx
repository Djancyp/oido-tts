import {useEffect, useRef, useState} from 'react';
import {Button} from '@/components/ui/button';
import {Textarea} from '@/components/ui/textarea';
import {
    Select,
    SelectContent,
    SelectGroup,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import {Globe, Mic, FolderOpen, Play, Pause, X} from 'lucide-react';
import {Events} from '@wailsio/runtime';
import {App as AppService, type ProgressEvent} from '../../bindings/oido-tts';
import {RecordVoiceDialog} from '@/components/RecordVoiceDialog';
import {glassPanel, formatTime} from '@/lib/ui';

const AUTO_LANG = 'auto';

const LANGS = [
    {code: AUTO_LANG, label: 'Model default'},
    {code: 'en', label: 'English'},
    {code: 'zh', label: 'Chinese'},
    {code: 'de', label: 'German'},
    {code: 'it', label: 'Italian'},
    {code: 'pt', label: 'Portuguese'},
    {code: 'es', label: 'Spanish'},
    {code: 'ja', label: 'Japanese'},
    {code: 'ko', label: 'Korean'},
    {code: 'fr', label: 'French'},
    {code: 'ru', label: 'Russian'},
];

// Fixed speaker labels the script must use — matches internal/tts.ParseScript's
// "Speaker: line" format exactly (case-sensitive map lookup on the backend).
const HOST = 'HOST';
const GUEST = 'GUEST';

const SCRIPT_PLACEHOLDER = `${HOST}: Welcome back to the show — today we're talking about something a little different.

${GUEST}: Wait, seriously? [surprised, a little excited]

${HOST}: Let's start from the beginning.`;

type Phase = 'idle' | 'ready' | 'working' | 'done' | 'error';

const STORAGE_KEY = 'oido-tts-podcast-settings';

function loadSettings(): {lang: string; hostVoice: string; guestVoice: string; outputDir: string} {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return {lang: AUTO_LANG, hostVoice: '', guestVoice: '', outputDir: ''};
        const parsed = JSON.parse(raw);
        return {
            lang: parsed.lang ?? AUTO_LANG,
            hostVoice: parsed.hostVoice ?? '',
            guestVoice: parsed.guestVoice ?? '',
            outputDir: parsed.outputDir ?? '',
        };
    } catch {
        return {lang: AUTO_LANG, hostVoice: '', guestVoice: '', outputDir: ''};
    }
}

function saveSettings(settings: {lang: string; hostVoice: string; guestVoice: string; outputDir: string}) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
    } catch {
        // best-effort; per-viewer convenience only
    }
}

export function PodcastView() {
    const initial = loadSettings();
    const [script, setScript] = useState('');
    const [lang, setLang] = useState(initial.lang);
    const [hostVoice, setHostVoice] = useState(initial.hostVoice);
    const [guestVoice, setGuestVoice] = useState(initial.guestVoice);
    const [outputDir, setOutputDir] = useState(initial.outputDir);
    const [audioUrl, setAudioUrl] = useState('');
    const [savedPath, setSavedPath] = useState('');
    const [errorMsg, setErrorMsg] = useState('');
    const [busy, setBusy] = useState(false);
    const [isPlaying, setIsPlaying] = useState(false);
    const [currentTime, setCurrentTime] = useState(0);
    const [duration, setDuration] = useState(0);
    const [progress, setProgress] = useState(0);
    const [recorderTarget, setRecorderTarget] = useState<'host' | 'guest' | null>(null);

    const audioRef = useRef<HTMLAudioElement>(null);

    useEffect(() => {
        saveSettings({lang, hostVoice, guestVoice, outputDir});
    }, [lang, hostVoice, guestVoice, outputDir]);

    useEffect(() => {
        return Events.On('podcast:progress', (e: {data: ProgressEvent}) => {
            setProgress(e.data.percent);
        });
    }, []);

    const phase: Phase = busy
        ? 'working'
        : errorMsg
            ? 'error'
            : audioUrl
                ? 'done'
                : script.trim()
                    ? 'ready'
                    : 'idle';

    async function pickVoice(target: 'host' | 'guest') {
        const path = await AppService.PickSpeakerFile();
        if (!path) return;
        if (target === 'host') setHostVoice(path);
        else setGuestVoice(path);
    }

    async function pickOutputDir() {
        const path = await AppService.PickOutputFolder();
        if (path) setOutputDir(path);
    }

    async function build() {
        if (!script.trim() || busy) return;
        setBusy(true);
        setErrorMsg('');
        setProgress(0);
        try {
            const result = await AppService.BuildPodcast(
                script,
                {[HOST]: hostVoice, [GUEST]: guestVoice},
                lang === AUTO_LANG ? '' : lang,
                outputDir
            );
            if (!result) throw new Error('No result returned');
            setAudioUrl(`data:audio/wav;base64,${result.audioB64}`);
            setSavedPath(result.savedPath ?? '');
        } catch (err) {
            if (!String(err).includes('stopped')) {
                setErrorMsg(String(err));
            }
        } finally {
            setBusy(false);
        }
    }

    async function stopBuild() {
        await AppService.StopSynthesis();
    }

    function buildAgain() {
        setAudioUrl('');
        setSavedPath('');
        setErrorMsg('');
        setIsPlaying(false);
        setCurrentTime(0);
        setDuration(0);
    }

    function togglePlay() {
        const el = audioRef.current;
        if (!el) return;
        if (el.paused) el.play();
        else el.pause();
    }

    return (
        <div className="flex flex-1 gap-4 overflow-hidden p-4">
            <aside className={glassPanel('flex w-72 shrink-0 flex-col gap-6 p-5')}>
                <div>
                    <h2 className="text-sm font-semibold text-foreground">Podcast settings</h2>
                    <p className="mt-0.5 text-xs text-muted-foreground">One language, two voices</p>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <Globe className="size-3.5" />
                        Language
                    </label>
                    <Select value={lang} onValueChange={setLang}>
                        <SelectTrigger className="w-full bg-white/70">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectGroup>
                                {LANGS.map((l) => (
                                    <SelectItem key={l.code} value={l.code}>
                                        {l.label}
                                    </SelectItem>
                                ))}
                            </SelectGroup>
                        </SelectContent>
                    </Select>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <Mic className="size-3.5" />
                        HOST voice
                    </label>
                    <div className="flex gap-1.5">
                        <Button
                            variant="outline"
                            className="w-full min-w-0 flex-1 justify-start truncate bg-white/70 font-normal"
                            onClick={() => pickVoice('host')}
                            title={hostVoice}
                        >
                            {hostVoice ? hostVoice.split('/').pop() : 'None — use default voice'}
                        </Button>
                        {hostVoice && (
                            <Button
                                variant="outline"
                                size="icon"
                                className="shrink-0 bg-white/70"
                                onClick={() => setHostVoice('')}
                                title="Clear — use default voice"
                                aria-label="Clear HOST voice"
                            >
                                <X className="size-3.5" />
                            </Button>
                        )}
                    </div>
                    <Button variant="ghost" size="sm" className="w-full justify-start text-muted-foreground" onClick={() => setRecorderTarget('host')}>
                        <Mic data-icon="inline-start" className="size-3.5" />
                        Record a clip instead
                    </Button>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <Mic className="size-3.5" />
                        GUEST voice
                    </label>
                    <div className="flex gap-1.5">
                        <Button
                            variant="outline"
                            className="w-full min-w-0 flex-1 justify-start truncate bg-white/70 font-normal"
                            onClick={() => pickVoice('guest')}
                            title={guestVoice}
                        >
                            {guestVoice ? guestVoice.split('/').pop() : 'None — use default voice'}
                        </Button>
                        {guestVoice && (
                            <Button
                                variant="outline"
                                size="icon"
                                className="shrink-0 bg-white/70"
                                onClick={() => setGuestVoice('')}
                                title="Clear — use default voice"
                                aria-label="Clear GUEST voice"
                            >
                                <X className="size-3.5" />
                            </Button>
                        )}
                    </div>
                    <Button variant="ghost" size="sm" className="w-full justify-start text-muted-foreground" onClick={() => setRecorderTarget('guest')}>
                        <Mic data-icon="inline-start" className="size-3.5" />
                        Record a clip instead
                    </Button>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <FolderOpen className="size-3.5" />
                        Output folder
                    </label>
                    <Button
                        variant="outline"
                        className="w-full justify-start truncate bg-white/70 font-normal"
                        onClick={pickOutputDir}
                        title={outputDir}
                    >
                        {outputDir || 'Playback only — nothing saved'}
                    </Button>
                </div>

                <div className="mt-auto border-t border-black/5 pt-4 text-[11px] leading-relaxed text-muted-foreground">
                    Runs fully on-device. No text leaves this machine.
                </div>
            </aside>

            <main className={glassPanel('flex flex-1 flex-col gap-4 p-6')}>
                <div className="flex items-center justify-between">
                    <h1 className="text-lg font-semibold tracking-tight text-foreground">Podcast</h1>
                    <Button
                        disabled={!script.trim() || busy}
                        onClick={build}
                        className="relative overflow-hidden rounded-full px-6 active:scale-95"
                    >
                        {busy && <span className="absolute inset-0 animate-pulse bg-white/20" />}
                        <span className="relative">{busy ? 'Building…' : 'Build episode'}</span>
                    </Button>
                </div>

                <Textarea
                    value={script}
                    onChange={(e) => setScript(e.target.value)}
                    placeholder={SCRIPT_PLACEHOLDER}
                    className="min-h-40 flex-1 resize-none bg-white/70 font-mono text-[14px] leading-relaxed"
                />
                <p className="-mt-2 text-[11px] text-muted-foreground">
                    One turn per paragraph, each starting with <code className="rounded bg-black/5 px-1">HOST:</code> or{' '}
                    <code className="rounded bg-black/5 px-1">GUEST:</code>. Add a style/emotion hint for one turn with a
                    trailing <code className="rounded bg-black/5 px-1">[emotion]</code> tag, e.g.{' '}
                    <code className="rounded bg-black/5 px-1">Wait, seriously? [surprised]</code> — a trailing bracket is
                    always treated as a tag and won't be spoken, so avoid ending a line with one for another reason (a
                    citation, a sound cue).
                </p>

                <div className="min-h-[6.5rem] rounded-xl border border-black/5">
                    {phase === 'idle' || phase === 'ready' ? (
                        <div className="flex h-[6.5rem] items-center justify-center text-sm text-muted-foreground">
                            Your episode will appear here
                        </div>
                    ) : phase === 'working' ? (
                        <div className="flex h-[6.5rem] flex-col items-center justify-center gap-2 px-8">
                            <div className="h-1.5 w-full max-w-sm overflow-hidden rounded-full bg-black/10">
                                <div
                                    className="h-full rounded-full bg-[#0066cc] transition-[width] duration-200"
                                    style={{width: `${Math.max(progress, 4)}%`}}
                                />
                            </div>
                            <div className="flex items-center gap-3">
                                <p className="text-xs text-muted-foreground">Building… {Math.round(progress)}%</p>
                                <button onClick={stopBuild} className="text-xs font-medium text-red-600 underline underline-offset-2">
                                    Stop
                                </button>
                            </div>
                        </div>
                    ) : phase === 'error' ? (
                        <div className="flex h-full flex-col items-center justify-center gap-2 rounded-xl bg-red-500/8 px-6 py-4 text-center">
                            <p className="text-sm font-semibold text-red-700">Couldn't build the episode.</p>
                            <p className="max-w-md text-xs text-red-700/80">{errorMsg}</p>
                            <button onClick={buildAgain} className="text-xs font-medium text-[#0066cc] underline underline-offset-2">
                                Try again
                            </button>
                        </div>
                    ) : (
                        <div className="flex h-full items-center gap-4 rounded-xl bg-[#1d1d1f] px-5 py-4 text-white shadow-[3px_5px_30px_0_rgba(0,0,0,0.22)]">
                            <button
                                onClick={togglePlay}
                                className="flex size-8 shrink-0 items-center justify-center rounded-full bg-white/10"
                                aria-label={isPlaying ? 'Pause' : 'Play'}
                            >
                                {isPlaying ? <Pause className="size-4" /> : <Play className="size-4" />}
                            </button>
                            <input
                                type="range"
                                min={0}
                                max={duration || 0}
                                value={currentTime}
                                onChange={(e) => {
                                    const t = Number(e.target.value);
                                    if (audioRef.current) audioRef.current.currentTime = t;
                                    setCurrentTime(t);
                                }}
                                className="h-1 flex-1 accent-[#2997ff]"
                            />
                            <span className="shrink-0 font-mono text-xs tabular-nums text-[#cccccc]">
                                {formatTime(currentTime)} / {formatTime(duration)}
                            </span>
                            <button onClick={buildAgain} className="shrink-0 text-xs font-medium text-[#2997ff]">
                                New
                            </button>
                            <audio
                                ref={audioRef}
                                src={audioUrl}
                                autoPlay
                                onPlay={() => setIsPlaying(true)}
                                onPause={() => setIsPlaying(false)}
                                onTimeUpdate={(e) => setCurrentTime(e.currentTarget.currentTime)}
                                onLoadedMetadata={(e) => setDuration(e.currentTarget.duration)}
                                onEnded={() => setIsPlaying(false)}
                                className="hidden"
                            />
                        </div>
                    )}
                </div>

                {phase === 'done' && savedPath && (
                    <p className="text-xs text-muted-foreground">Saved to {savedPath}</p>
                )}
            </main>

            <RecordVoiceDialog
                open={recorderTarget !== null}
                onOpenChange={(open) => {
                    if (!open) setRecorderTarget(null);
                }}
                onRecorded={(path) => {
                    if (recorderTarget === 'host') setHostVoice(path);
                    else if (recorderTarget === 'guest') setGuestVoice(path);
                    setRecorderTarget(null);
                }}
            />
        </div>
    );
}
