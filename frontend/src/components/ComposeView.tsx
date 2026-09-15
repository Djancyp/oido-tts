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
import {Globe, Mic, FolderOpen, Play, Pause, Sparkles, X} from 'lucide-react';
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

type Phase = 'idle' | 'composing' | 'working' | 'done' | 'error';

const STORAGE_KEY = 'oido-tts-settings';

function loadSettings(): {lang: string; speakerFile: string; outputDir: string; instruct: string} {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return {lang: AUTO_LANG, speakerFile: '', outputDir: '', instruct: ''};
        const parsed = JSON.parse(raw);
        return {
            lang: parsed.lang ?? AUTO_LANG,
            speakerFile: parsed.speakerFile ?? '',
            outputDir: parsed.outputDir ?? '',
            instruct: parsed.instruct ?? '',
        };
    } catch {
        return {lang: AUTO_LANG, speakerFile: '', outputDir: '', instruct: ''};
    }
}

function saveSettings(settings: {lang: string; speakerFile: string; outputDir: string; instruct: string}) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
    } catch {
        // best-effort; per-viewer convenience only
    }
}

export function ComposeView() {
    const initial = loadSettings();
    const [text, setText] = useState('');
    const [lang, setLang] = useState(initial.lang);
    const [speakerFile, setSpeakerFile] = useState(initial.speakerFile);
    const [outputDir, setOutputDir] = useState(initial.outputDir);
    const [instruct, setInstruct] = useState(initial.instruct);
    const [audioUrl, setAudioUrl] = useState('');
    const [savedPath, setSavedPath] = useState('');
    const [errorMsg, setErrorMsg] = useState('');
    const [busy, setBusy] = useState(false);
    const [isPlaying, setIsPlaying] = useState(false);
    const [currentTime, setCurrentTime] = useState(0);
    const [duration, setDuration] = useState(0);
    const [recorderOpen, setRecorderOpen] = useState(false);
    const [progress, setProgress] = useState(0);

    const audioRef = useRef<HTMLAudioElement>(null);

    useEffect(() => {
        saveSettings({lang, speakerFile, outputDir, instruct});
    }, [lang, speakerFile, outputDir, instruct]);

    useEffect(() => {
        return Events.On('tts:progress', (e: {data: ProgressEvent}) => {
            setProgress(e.data.percent);
        });
    }, []);

    const phase: Phase = busy
        ? 'working'
        : errorMsg
            ? 'error'
            : audioUrl
                ? 'done'
                : text.trim()
                    ? 'composing'
                    : 'idle';

    async function pickSpeaker() {
        const path = await AppService.PickSpeakerFile();
        if (path) setSpeakerFile(path);
    }

    async function pickOutputDir() {
        const path = await AppService.PickOutputFolder();
        if (path) setOutputDir(path);
    }

    async function speak() {
        if (!text.trim() || busy) return;
        setBusy(true);
        setErrorMsg('');
        setProgress(0);
        try {
            const result = await AppService.Synthesize(text, lang === AUTO_LANG ? '' : lang, speakerFile, instruct, outputDir);
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

    async function stopSynthesis() {
        await AppService.StopSynthesis();
    }

    function speakAgain() {
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
        if (el.paused) {
            el.play();
        } else {
            el.pause();
        }
    }

    return (
        <div className="flex flex-1 gap-4 overflow-hidden p-4">
            <aside className={glassPanel('flex w-72 shrink-0 flex-col gap-6 p-5')}>
                <div>
                    <h2 className="text-sm font-semibold text-foreground">Voice settings</h2>
                    <p className="mt-0.5 text-xs text-muted-foreground">Applies to every request</p>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <Globe className="size-3.5" />
                        Language
                    </label>
                    <Select value={lang} onValueChange={setLang}>
                        <SelectTrigger className="w-full bg-muted">
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
                        Voice to clone
                    </label>
                    <div className="flex gap-1.5">
                        <Button
                            variant="outline"
                            className="w-full min-w-0 flex-1 justify-start truncate bg-muted font-normal"
                            onClick={pickSpeaker}
                            title={speakerFile}
                        >
                            {speakerFile ? speakerFile.split('/').pop() : 'None — use default voice'}
                        </Button>
                        {speakerFile && (
                            <Button
                                variant="outline"
                                size="icon"
                                className="shrink-0 bg-muted"
                                onClick={() => setSpeakerFile('')}
                                title="Clear — use default voice"
                                aria-label="Clear voice to clone"
                            >
                                <X className="size-3.5" />
                            </Button>
                        )}
                    </div>
                    <Button variant="ghost" size="sm" className="w-full justify-start text-muted-foreground" onClick={() => setRecorderOpen(true)}>
                        <Mic data-icon="inline-start" className="size-3.5" />
                        Record a clip instead
                    </Button>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <Sparkles className="size-3.5" />
                        Default style (optional)
                    </label>
                    <Textarea
                        value={instruct}
                        onChange={(e) => setInstruct(e.target.value)}
                        placeholder="e.g. speak with a hint of panic creeping into your voice"
                        maxLength={500}
                        className="min-h-16 resize-none bg-muted text-xs"
                    />
                    <p className="text-[11px] leading-relaxed text-muted-foreground">
                        Used until a <code className="rounded bg-border px-1">[emotion]</code> tag in the text below
                        takes over — write one inline, e.g. <code className="rounded bg-border px-1">...you won't
                        believe it! [excited]</code>, and everything after it uses that style instead, until the next
                        tag. Add <code className="rounded bg-border px-1">[pause:2s]</code> anywhere to insert a
                        timed silence and continue — <code className="rounded bg-border px-1">ms</code> or{" "}
                        <code className="rounded bg-border px-1">s</code> units, up to 30s.
                    </p>
                </div>

                <div className="flex flex-col gap-2">
                    <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        <FolderOpen className="size-3.5" />
                        Output folder
                    </label>
                    <Button
                        variant="outline"
                        className="w-full justify-start truncate bg-muted font-normal"
                        onClick={pickOutputDir}
                        title={outputDir}
                    >
                        {outputDir || 'Playback only — nothing saved'}
                    </Button>
                </div>

                <div className="mt-auto border-t border-border pt-4 text-[11px] leading-relaxed text-muted-foreground">
                    Runs fully on-device. No text leaves this machine.
                </div>
            </aside>

            <main className={glassPanel('flex flex-1 flex-col gap-4 p-6')}>
                <div className="flex items-center justify-between">
                    <h1 className="text-lg font-semibold tracking-tight text-foreground">Compose</h1>
                    <Button
                        disabled={!text.trim() || busy}
                        onClick={speak}
                        className="relative overflow-hidden rounded-lg px-6 active:scale-95"
                    >
                        {busy && <span className="absolute inset-0 animate-pulse bg-white/20" />}
                        <span className="relative">{busy ? 'Speaking…' : 'Speak'}</span>
                    </Button>
                </div>

                <Textarea
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    placeholder="The quick brown fox jumps over the lazy dog."
                    className="min-h-40 flex-1 resize-none bg-muted text-[15px] leading-relaxed"
                    autoFocus
                />

                <div className="min-h-[6.5rem] rounded-xl border border-border">
                    {phase === 'idle' || phase === 'composing' ? (
                        <div className="flex h-[6.5rem] items-center justify-center text-sm text-muted-foreground">
                            Your audio will appear here
                        </div>
                    ) : phase === 'working' ? (
                        <div className="flex h-[6.5rem] flex-col items-center justify-center gap-2 px-8">
                            <div className="h-1.5 w-full max-w-sm overflow-hidden rounded-full bg-muted">
                                <div
                                    className="h-full rounded-full bg-primary transition-[width] duration-200"
                                    style={{width: `${Math.max(progress, 4)}%`}}
                                />
                            </div>
                            <div className="flex items-center gap-3">
                                <p className="text-xs text-muted-foreground">Generating… {Math.round(progress)}%</p>
                                <button onClick={stopSynthesis} className="text-xs font-medium text-destructive underline underline-offset-2">
                                    Stop
                                </button>
                            </div>
                        </div>
                    ) : phase === 'error' ? (
                        <div className="flex h-full flex-col items-center justify-center gap-2 rounded-xl bg-destructive/8 px-6 py-4 text-center">
                            <p className="text-sm font-semibold text-destructive">Couldn't generate audio.</p>
                            <p className="max-w-md text-xs text-destructive/80">{errorMsg}</p>
                            <button onClick={speakAgain} className="text-xs font-medium text-primary underline underline-offset-2">
                                Try again
                            </button>
                        </div>
                    ) : (
                        <div className="flex h-full items-center gap-4 rounded-xl bg-secondary px-5 py-4 text-secondary-foreground shadow-[0_4px_20px_rgba(28,27,26,0.18)]">
                            <button
                                onClick={togglePlay}
                                className="flex size-8 shrink-0 items-center justify-center rounded-full bg-secondary-foreground/10"
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
                                className="h-1 flex-1 accent-primary"
                            />
                            <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
                                {formatTime(currentTime)} / {formatTime(duration)}
                            </span>
                            <button onClick={speakAgain} className="shrink-0 text-xs font-medium text-primary">
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
                open={recorderOpen}
                onOpenChange={setRecorderOpen}
                onRecorded={(path) => setSpeakerFile(path)}
            />
        </div>
    );
}
