import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
  type Dispatch,
  type SetStateAction,
} from 'react'
export type PlayerActions = { like: () => void; comments: () => void }
export type MuteState = [boolean, Dispatch<SetStateAction<boolean>>]
export function usePlayerControls(
  active: boolean,
  navigate?: (step: number) => void,
  muteState?: MuteState,
) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const actionsRef = useRef<PlayerActions | null>(null)
  const localMute = useState(true)
  const [muted, setMuted] = muteState ?? localMute
  function togglePlay() {
    const video = videoRef.current
    if (!video) return
    if (video.paused) void video.play().catch(() => {})
    else video.pause()
  }
  function onVideoClick(event: MouseEvent<HTMLVideoElement>) {
    // Preserve the native transport controls at the bottom of the video.
    if (event.clientY > event.currentTarget.getBoundingClientRect().bottom - 48) return
    event.preventDefault()
    togglePlay()
  }
  useEffect(() => {
    if (!active) return
    function onKeyDown(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null
      if (
        event.altKey ||
        event.ctrlKey ||
        event.metaKey ||
        target?.closest('input, textarea, select, button, a, [contenteditable], [role="dialog"]') ||
        document.querySelector('[role="dialog"]')
      )
        return
      const key = event.key.toLowerCase()
      if (
        key === ' ' ||
        key === 'm' ||
        key === 'c' ||
        (navigate && (key === 'arrowdown' || key === 'arrowup'))
      ) {
        event.preventDefault()
        if (key === ' ') togglePlay()
        if (key === 'm') setMuted((value) => !value)
        if (key === 'c') actionsRef.current?.comments()
        if (key === 'arrowdown') navigate?.(1)
        if (key === 'arrowup') navigate?.(-1)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [active, navigate, setMuted])
  return { videoRef, actionsRef, muted, setMuted, onVideoClick }
}
