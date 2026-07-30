import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import ActiveEventPanel from './ActiveEventPanel.jsx'

describe('ActiveEventPanel', () => {
  it('renders the current-state tree and delegates collapse', async () => {
    const onToggle = vi.fn()
    render(<ActiveEventPanel open item={{ currentState: 'IDLE', processingEvent: 'MOVE', availableTransitions: [
      { fromState: 'IDLE', event: 'RUN', toState: 'RUN' },
      { fromState: 'IDLE', event: 'MOVE', toState: 'WALK' },
    ] }} onToggle={onToggle} />)

    expect(screen.getByRole('tree', { name: 'IDLE 可接收事件' })).toHaveTextContent('MOVE→WALK')
    expect(screen.getByText('PROCESSING')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '折叠 Active Event' }))
    expect(onToggle).toHaveBeenCalledWith(false)
  })
})
