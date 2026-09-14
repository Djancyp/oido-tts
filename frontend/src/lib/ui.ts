// Flat panel per DESIGN.md section 4: Panel Surface fill, Seam border, whisper
// shadow tinted toward Ink — no blur/glass, no glow.
export function glassPanel(extra = ''): string {
    return 'rounded-xl border border-border bg-card shadow-[0_1px_3px_rgba(28,27,26,0.06)] ' + extra;
}

export function formatTime(seconds: number): string {
    if (!Number.isFinite(seconds)) return '0:00';
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
}
