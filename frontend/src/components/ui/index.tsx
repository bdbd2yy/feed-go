import {
  useId,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
} from 'react'
import { Dialog } from '@base-ui/react/dialog'
import { Tabs as BaseTabs } from '@base-ui/react/tabs'
import { cva, type VariantProps } from 'class-variance-authority'
import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { CircleNotchIcon, XIcon, FilmSlateIcon } from '@phosphor-icons/react'

export function cn(...values: ClassValue[]) {
  return twMerge(clsx(values))
}
const button = cva('ui-button', {
  variants: {
    variant: {
      primary: 'ui-button-primary',
      secondary: 'ui-button-secondary',
      ghost: 'ui-button-ghost',
      danger: 'ui-button-danger',
    },
    size: { default: '', small: 'ui-button-small', icon: 'ui-button-icon' },
  },
  defaultVariants: { variant: 'secondary', size: 'default' },
})
export function Button({
  variant,
  size,
  busy,
  className,
  disabled,
  children,
  type = 'button',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof button> & { busy?: boolean }) {
  return (
    <button
      {...props}
      type={type}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={cn(button({ variant, size }), className)}
    >
      {busy && <CircleNotchIcon className="spin" />}
      {children}
    </button>
  )
}
export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn('ui-input', className)} {...props} />
}
export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn('ui-input ui-textarea', className)} {...props} />
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: (id: string) => ReactNode
}) {
  const id = useId()
  return (
    <div className="ui-field">
      <label htmlFor={id}>{label}</label>
      {children(id)}
      {hint && <p className="text-muted text-xs">{hint}</p>}
    </div>
  )
}
export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={cn('ui-card', className)}>{children}</section>
}
export function Badge({ children }: { children: ReactNode }) {
  return <span className="ui-badge">{children}</span>
}
export function Avatar({
  name,
  src,
  large = false,
}: {
  name: string
  src?: string
  large?: boolean
}) {
  return (
    <span className={cn('ui-avatar', large && 'ui-avatar-large')}>
      {src ? <img src={src} alt={name} /> : name.slice(0, 1).toUpperCase()}
    </span>
  )
}
export function EmptyState({
  title,
  description,
  action,
}: {
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <div className="ui-empty">
      <span className="ui-empty-icon">
        <FilmSlateIcon size={26} />
      </span>
      <h2>{title}</h2>
      {description && <p>{description}</p>}
      {action}
    </div>
  )
}
export function QueryState({
  pending,
  error,
  retry,
}: {
  pending?: boolean
  error?: Error | null
  retry?: () => void
}) {
  if (pending)
    return (
      <div role="status" className="loading">
        <CircleNotchIcon className="spin" size={22} />
        正在加载…
      </div>
    )
  if (error)
    return (
      <div role="alert" className="error-state">
        <p>{error.message || '加载失败，请稍后重试'}</p>
        {retry && (
          <Button size="small" onClick={retry}>
            重新加载
          </Button>
        )}
      </div>
    )
  return null
}
export function Modal({
  open,
  onOpenChange,
  title,
  children,
  drawer = false,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  children: ReactNode
  drawer?: boolean
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="dialog-backdrop" />
        <Dialog.Popup className={cn('dialog-popup', drawer && 'dialog-drawer')}>
          <div className="dialog-heading">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close className="ui-button ui-button-ghost ui-button-icon" aria-label="关闭">
              <XIcon size={18} />
            </Dialog.Close>
          </div>
          {children}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
export function Tabs({
  value,
  onChange,
  items,
  children,
}: {
  value: string
  onChange: (value: string) => void
  items: { value: string; label: string }[]
  children: ReactNode
}) {
  return (
    <BaseTabs.Root
      value={value}
      onValueChange={(v) => {
        if (typeof v === 'string') onChange(v)
      }}
    >
      <BaseTabs.List className="ui-tabs">
        {items.map((item) => (
          <BaseTabs.Tab className="ui-tab" value={item.value} key={item.value}>
            {item.label}
          </BaseTabs.Tab>
        ))}
      </BaseTabs.List>
      {items.map((item) => (
        <BaseTabs.Panel value={item.value} key={item.value}>
          {value === item.value ? children : null}
        </BaseTabs.Panel>
      ))}
    </BaseTabs.Root>
  )
}
