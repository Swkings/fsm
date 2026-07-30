export function machineName(item) { return item.displayName || item.name || item.id }

export function compareMachines(left, right) {
  return String(machineName(left)).localeCompare(String(machineName(right)), 'zh-CN')
}

export function buildFolderTree(items) {
  const root = { name: '', path: '', folders: new Map(), items: [] }
  items.forEach((item) => {
    let node = root
    String(item.path || '').split('/').map((part) => part.trim()).filter(Boolean).forEach((part) => {
      const path = node.path ? `${node.path}/${part}` : part
      if (!node.folders.has(part)) node.folders.set(part, { name: part, path, folders: new Map(), items: [] })
      node = node.folders.get(part)
    })
    node.items.push(item)
  })
  return root
}

export function folderSize(node) {
  return node.items.length + [...node.folders.values()].reduce((total, child) => total + folderSize(child), 0)
}
