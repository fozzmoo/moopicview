import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './dialog'

function TestDialog() {
  return (
    <Dialog open>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Test Dialog</DialogTitle>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  )
}

describe('Dialog', () => {
  it('dialog overlay uses fully opaque background without transparency', () => {
    render(<TestDialog />)

    const overlay = document.querySelector('[data-state="open"][class*="fixed inset-0"]')
    expect(overlay).toBeInTheDocument()
    const classes = overlay!.className
    expect(classes).toContain('bg-black')
    expect(classes).not.toContain('bg-black/')
  })

  it('dialog content has opaque background', () => {
    render(<TestDialog />)

    const dialog = screen.getByRole('dialog')
    const classes = dialog.className
    expect(classes).toContain('bg-background')
  })
})
