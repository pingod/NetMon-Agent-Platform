import { lazyRetry } from '@/lib/lazyRetry';

const Servers = lazyRetry(() => import('./pages/Servers'));
const VirtualMachines = lazyRetry(() => import('./pages/VirtualMachinesPage').then(m => ({ default: m.VirtualMachinesPage })));
const SSHKeys = lazyRetry(() => import('./pages/SSHKeys'));
const TerminalPage = lazyRetry(() => import('./pages/TerminalPage'));
const RemoteDesktop = lazyRetry(() => import('./pages/RemoteDesktop'));

export const serverRoutes = [
  { path: 'servers', element: Servers },
  { path: 'virtual-machines', element: VirtualMachines },
  { path: 'ssh-keys', element: SSHKeys },
  { path: 'terminal', element: TerminalPage },
  { path: 'remote-desktop', element: RemoteDesktop },
  { path: 'remote-desktop/:serverId', element: RemoteDesktop },
];
