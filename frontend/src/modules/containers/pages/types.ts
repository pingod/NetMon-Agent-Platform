// ── Types ──────────────────────────────────────────────

export type Tab = 'containers' | 'images' | 'volumes' | 'networks' | 'endpoints';

export interface EndpointHost {
  id: string;
  name: string;
  host: string;
  port?: number;
  protocol?: string;
  status: string;
}

export interface ContainerItem {
  id: string;
  Names?: string[];
  name?: string;
  Image?: string;
  image?: string;
  State?: string;
  state?: string;
  Status?: string;
  status?: string;
  Ports?: Array<{ PublicPort?: number; PrivatePort?: number; Type?: string }>;
  Created?: number;
  created?: number;
}

export interface ImageItem {
  Id?: string;
  id?: string;
  RepoTags?: string[];
  RepoDigests?: string[];
  Size?: number;
  Created?: number;
}

export interface VolumeItem {
  Name?: string;
  name?: string;
  Driver?: string;
  driver?: string;
  Mountpoint?: string;
  mountpoint?: string;
  CreatedAt?: string;
  createdAt?: string;
}

export interface NetworkItem {
  Id?: string;
  id?: string;
  Name?: string;
  name?: string;
  Driver?: string;
  driver?: string;
  Scope?: string;
  scope?: string;
  IPAM?: { Driver?: string; Config?: Array<{ Subnet?: string; Gateway?: string }> };
  Containers?: Record<string, { Name: string; IPv4Address: string }>;
  containers?: Record<string, { Name: string; IPv4Address: string }>;
}

export interface EndpointItem {
  id: string;
  name: string;
  host: string;
  port: number;
  protocol: string;
  status: string;
  tlsCa?: string;
  tlsCert?: string;
  tlsKey?: string;
  error_message?: string;
}

// ── Helpers ────────────────────────────────────────────

export function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

export function formatDate(ts: number | string | undefined): string {
  if (!ts) return '-';
  const d = new Date(typeof ts === 'string' ? ts : ts * 1000);
  return d.toLocaleString('zh-CN');
}

export function statusBadge(status: string): { bg: string; text: string; dot: string } {
  const s = status?.toLowerCase() || '';
  if (s === 'running' || s === 'active' || s === 'up') return { bg: 'bg-green-500/10 border-green-500/30', text: 'text-green-400', dot: 'bg-green-500' };
  if (s === 'stopped' || s === 'exited' || s === 'inactive') return { bg: 'bg-red-500/10 border-red-500/30', text: 'text-red-400', dot: 'bg-red-500' };
  if (s === 'paused') return { bg: 'bg-yellow-500/10 border-yellow-500/30', text: 'text-yellow-400', dot: 'bg-yellow-500' };
  if (s === 'error') return { bg: 'bg-red-500/10 border-red-500/30', text: 'text-red-400', dot: 'bg-red-500' };
  return { bg: 'bg-gray-500/10 border-gray-500/30', text: 'text-gray-400', dot: 'bg-gray-500' };
}

export function containerName(c: ContainerItem): string {
  const name = (c.Names?.[0] || c.name || '').replace(/^\//, '');
  return name || c.id?.substring(0, 12) || '-';
}

export function imageRepo(img: ImageItem): string {
  const tag = img.RepoTags?.[0] || '';
  const idx = tag.lastIndexOf(':');
  return idx > 0 ? tag.substring(0, idx) : tag || '<none>';
}

export function imageTagOnly(img: ImageItem): string {
  const tag = img.RepoTags?.[0] || '';
  const idx = tag.lastIndexOf(':');
  return idx > 0 ? tag.substring(idx + 1) : 'latest';
}

export function withEndpointParams(endpointId: string, params?: Record<string, unknown>): Record<string, unknown> {
  return endpointId !== 'local'
    ? { ...params, endpointId }
    : { ...params };
}
