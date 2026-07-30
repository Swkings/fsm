import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import Sidebar from './Sidebar.jsx'

describe('Sidebar', () => {
  it('filters nested machines and selects the result', async () => {
    const onSelect = vi.fn()
    render(<Sidebar fsms={[
      { id: 'player', name: 'player', displayName: 'Player FSM', path: 'game/actors', currentState: 'IDLE', mode: 'push' },
      { id: 'door', name: 'door', path: 'world', currentState: 'OPEN', mode: 'pull' },
    ]} selected="" onSelect={onSelect} collapsed={false} onToggle={() => {}} />)

    await userEvent.type(screen.getByRole('searchbox', { name: '搜索 FSM' }), 'player')
    expect(screen.queryByText('door')).not.toBeInTheDocument()
    await userEvent.click(screen.getByText('Player FSM'))
    expect(onSelect).toHaveBeenCalledWith('player')
  })
})
