/* The options API marks profiles that only apply to plain KCP. */
export function presetsForTransport(presets, transport) {
  return (presets || []).filter(p => !p.kcpOnly || transport === 'kcp');
}
