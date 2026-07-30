import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import TracePanel from './TracePanel.jsx'

const traces = [
  { transitionId: '1', beginTime: 1_000_000_000, duration: 10_000, event: 'MOVE', fromState: 'IDLE', finalState: 'WALK', success: true, entryMetadata: { payload: { hp: 10 } }, exitMetadata: { payload: { hp: 10 } } },
  { transitionId: '2', beginTime: 2_000_000_000, duration: 20_000, event: 'DAMAGE', fromState: 'WALK', finalState: 'HURT', success: false, error: 'blocked', entryMetadata: { payload: { hp: 10 } }, exitMetadata: { payload: { hp: 5 } } },
]

describe('TracePanel', () => {
  it('expands, filters, and opens FSM metadata from the context menu', async () => {
    render(<TracePanel traces={traces} />)
    await userEvent.click(screen.getByRole('button', { name: '展开完整 Trace' }))
    await userEvent.type(screen.getByRole('searchbox', { name: '搜索 Trace' }), 'DAMAGE')
    const damageCell = screen.getAllByText('DAMAGE').find((element) => element.tagName === 'TD')
    expect(damageCell).toBeInTheDocument()
    expect(screen.queryByText('MOVE')).not.toBeInTheDocument()

    fireEvent.contextMenu(damageCell.closest('tr'), { clientX: 20, clientY: 20 })
    await userEvent.click(screen.getByRole('menuitem', { name: /查看 FSM 数据/ }))
    expect(screen.getByRole('dialog', { name: 'FSM 数据快照' })).toHaveTextContent('"hp": 5')
  })
})
