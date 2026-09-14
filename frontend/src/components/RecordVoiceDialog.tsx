import {useRef, useState} from 'react';
import {Button} from '@/components/ui/button';
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from '@/components/ui/dialog';
import {Mic, Square} from 'lucide-react';
import {App as AppService} from '../../bindings/oido-tts';

// A short, phonetically varied passage — enough speech for a clean voice
// reference (~8-12s read aloud) without asking for more than that.
const READING_PROMPT =
    'The quick brown fox jumps over the lazy dog. ' +
    'Peter Piper picked a peck of pickled peppers, and she sells seashells by the seashore.';

const MAX_SECONDS = 30;

type Stage = 'ready' | 'recording' | 'recorded' | 'saving' | 'error';

export function RecordVoiceDialog({
    open,
    onOpenChange,
    onRecorded,
}: {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onRecorded: (path: string) => void;
}) {
    const [stage, setStage] = useState<Stage>('ready');
    const [elapsed, setElapsed] = useState(0);
    const [previewUrl, setPreviewUrl] = useState('');
    const [errorMsg, setErrorMsg] = useState('');

    const timerRef = useRef<number | null>(null);

    function clearTimer() {
        if (timerRef.current !== null) {
            window.clearInterval(timerRef.current);
            timerRef.current = null;
        }
    }

    function reset() {
        setStage('ready');
        setElapsed(0);
        setErrorMsg('');
        setPreviewUrl('');
    }

    async function startRecording() {
        setErrorMsg('');
        try {
            await AppService.StartRecording();
            setStage('recording');
            setElapsed(0);
            timerRef.current = window.setInterval(() => {
                setElapsed((s) => {
                    if (s + 1 >= MAX_SECONDS) stopRecording();
                    return s + 1;
                });
            }, 1000);
        } catch (err) {
            setErrorMsg(String(err));
            setStage('error');
        }
    }

    async function stopRecording() {
        clearTimer();
        try {
            const base64 = await AppService.StopRecording();
            setPreviewUrl(`data:audio/wav;base64,${base64}`);
            setStage('recorded');
        } catch (err) {
            setErrorMsg(String(err));
            setStage('error');
        }
    }

    async function recordAgain() {
        await AppService.DiscardRecording();
        reset();
    }

    async function useRecording() {
        setStage('saving');
        try {
            const path = await AppService.ConfirmRecording();
            onRecorded(path);
            handleClose();
        } catch (err) {
            setErrorMsg(String(err));
            setStage('error');
        }
    }

    async function handleClose() {
        clearTimer();
        if (stage === 'recorded' || stage === 'error') {
            await AppService.DiscardRecording();
        }
        reset();
        onOpenChange(false);
    }

    return (
        <Dialog open={open} onOpenChange={(o) => (o ? onOpenChange(o) : handleClose())}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Record a voice to clone</DialogTitle>
                    <DialogDescription>Read the passage below out loud, at a natural pace.</DialogDescription>
                </DialogHeader>

                <div className="rounded-lg border border-black/10 bg-black/[0.02] p-4 text-[15px] leading-relaxed text-foreground">
                    {READING_PROMPT}
                </div>

                {stage === 'ready' && (
                    <div className="flex flex-col items-center gap-3 py-2">
                        <Button size="lg" onClick={startRecording} className="rounded-full px-6">
                            <Mic data-icon="inline-start" />
                            Start recording
                        </Button>
                        <p className="text-xs text-muted-foreground">Up to {MAX_SECONDS}s — a few sentences is enough.</p>
                    </div>
                )}

                {stage === 'recording' && (
                    <div className="flex flex-col items-center gap-3 py-2">
                        <div className="flex items-center gap-2 text-red-600">
                            <span className="size-2 animate-pulse rounded-full bg-red-600" />
                            <span className="font-mono text-sm tabular-nums">0:{elapsed.toString().padStart(2, '0')}</span>
                        </div>
                        <Button size="lg" variant="outline" onClick={stopRecording} className="rounded-full px-6">
                            <Square data-icon="inline-start" className="fill-current" />
                            Stop
                        </Button>
                    </div>
                )}

                {stage === 'recorded' && (
                    <div className="flex flex-col items-center gap-3 py-2">
                        <audio controls src={previewUrl} className="w-full" />
                        <div className="flex gap-2">
                            <Button variant="outline" onClick={recordAgain}>
                                Record again
                            </Button>
                            <Button onClick={useRecording}>Use this recording</Button>
                        </div>
                    </div>
                )}

                {stage === 'saving' && (
                    <p className="py-4 text-center text-sm text-muted-foreground">Saving…</p>
                )}

                {stage === 'error' && (
                    <div className="flex flex-col items-center gap-3 py-2">
                        <p className="text-sm text-red-700">{errorMsg}</p>
                        <Button variant="outline" onClick={reset}>
                            Try again
                        </Button>
                    </div>
                )}

                <DialogFooter>
                    <Button variant="ghost" onClick={handleClose}>
                        Cancel
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
