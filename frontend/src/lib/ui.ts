export function glassPanel(extra = ''): string {
    return (
        'rounded-2xl border border-white/50 bg-white/45 shadow-[0_8px_32px_rgba(31,41,55,0.10)] ' +
        'backdrop-blur-xl backdrop-saturate-150 ' +
        extra
    );
}

export function formatTime(seconds: number): string {
    if (!Number.isFinite(seconds)) return '0:00';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
}
