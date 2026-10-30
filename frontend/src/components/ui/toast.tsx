import { useSyncExternalStore } from 'react'
import { CheckCircleIcon, WarningCircleIcon, XIcon } from '@phosphor-icons/react'
import { Button } from './index'
type Notice = { id: number; message: string; error: boolean }
let notices: Notice[] = []
let nextId = 0
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((fn) => fn())
function dismiss(id: number) {
  notices = notices.filter((item) => item.id !== id)
  emit()
}
export function notify(message: string, error = false) {
  const id = ++nextId
  notices = [...notices.slice(-3), { id, message, error }]
  emit()
  setTimeout(() => dismiss(id), 5000)
}
export function notifyError(error: unknown) {
  notify(error instanceof Error ? error.message : String(error), true)
}
function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}
export function Toaster() {
  const items = useSyncExternalStore(subscribe, () => notices)
  return (
    <div className="toaster" aria-live="polite">
      {items.map((item) => (
        <div className="toast" role={item.error ? 'alert' : 'status'} key={item.id}>
          {item.error ? (
            <WarningCircleIcon className="text-danger" size={20} />
          ) : (
            <CheckCircleIcon className="text-success" size={20} />
          )}
          <span>{item.message}</span>
          <Button
            variant="ghost"
            size="icon"
            aria-label="关闭通知"
            onClick={() => dismiss(item.id)}
          >
            <XIcon />
          </Button>
        </div>
      ))}
    </div>
  )
}
