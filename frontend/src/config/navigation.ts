import {
  LayoutDashboard,
  Monitor,
  Server,
  Network,
  Router,
  Radio,
  Globe,
  Database,
  Key,
  Terminal,
  MonitorPlay,
  Container,
  Box,
  Activity,
  HardDrive,
  Image as ImageIcon,
  Cpu,
  Layers,
  Camera,
  Package,
  DollarSign,
  TrendingUp,
  Building2,
  LayoutGrid,
  LayoutTemplate,
  Zap,
  Cog,
  ShieldCheck,
  FileText,
  AlertTriangle,
  Bell,
  Link2,
  Shield,
  Search,
  Brain,
  Lightbulb,
  ListChecks,
  BarChart3,
  BookMarked,
  BookOpen,
  MessageSquare,
  Users,
  FlaskConical,
  Settings,
  Home,
  ServerCog,
  ArchiveRestore,
  Radar,
  Tag,
  type LucideIcon,
} from 'lucide-react';

export type NavItem = {
  name: string;
  href: string;
  icon: LucideIcon;
};

export type NavGroup = {
  name: string;
  icon: LucideIcon;
  items: NavItem[];
};

/** 侧边栏导航分组配置 */
export const navigationGroups: NavGroup[] = [
  {
    name: 'nav.home',
    icon: Home,
    items: [
      { name: 'nav.dashboard', href: '/dashboard', icon: LayoutDashboard },
      { name: 'nav.topology', href: '/topology', icon: Network },
      { name: 'nav.networkDashboard', href: '/network-dashboard', icon: BarChart3 },
    ],
  },
  {
    name: 'nav.serverMgmt',
    icon: ServerCog,
    items: [
      { name: 'nav.servers', href: '/servers', icon: Server },
      { name: 'nav.virtualMachines', href: '/virtual-machines', icon: Cpu },
      { name: 'nav.networkDevices', href: '/network-devices', icon: Network },
      { name: 'nav.networks', href: '/networks', icon: Router },
      { name: 'nav.networkDiscovery', href: '/network-discovery', icon: Globe },
      { name: 'nav.configBackup', href: '/config-backup', icon: ArchiveRestore },
      { name: 'nav.snmpCollect', href: '/snmp-collect', icon: Radar },
      { name: 'nav.snmpCommunities', href: '/snmp-communities', icon: Key },
      { name: 'nav.metricTags', href: '/metric-tags', icon: Tag },
      { name: 'nav.dbConnections', href: '/db-connections', icon: Database },
      { name: 'nav.sshKeys', href: '/ssh-keys', icon: Key },
      { name: 'nav.terminal', href: '/terminal', icon: Terminal },
      { name: 'nav.remoteDesktop', href: '/remote-desktop', icon: MonitorPlay },
    ],
  },
  {
    name: 'nav.containersVirtualization',
    icon: Box,
    items: [
      { name: 'nav.containers', href: '/containers', icon: Container },
      { name: 'nav.containerMonitor', href: '/container-monitor', icon: Activity },
      { name: 'nav.containerLogs', href: '/container-logs', icon: Terminal },
      { name: 'nav.images', href: '/images', icon: ImageIcon },
      { name: 'nav.volumes', href: '/volumes', icon: HardDrive },
      { name: 'nav.virtualMachines', href: '/virtual-machines', icon: Cpu },
      { name: 'nav.compose', href: '/compose', icon: Layers },
      { name: 'nav.snapshotPolicies', href: '/snapshot-policies', icon: Camera },
      { name: 'nav.imageRegistry', href: '/image-registry', icon: Package },
      { name: 'nav.kubernetes', href: '/kubernetes', icon: Container },
      { name: 'nav.costAnalysis', href: '/cost-analysis', icon: DollarSign },
      { name: 'nav.autoScale', href: '/auto-scale', icon: TrendingUp },
    ],
  },
  {
    name: 'nav.dataCenter',
    icon: Building2,
    items: [
      { name: 'nav.dcInfrastructure', href: '/dc-manage', icon: LayoutGrid },
      { name: 'nav.dcRack2D', href: '/dc-rack-2d', icon: Server },
      { name: 'nav.dcRoom3D', href: '/data-room', icon: Monitor },
    ],
  },
  {
    name: 'nav.templateCenter',
    icon: LayoutTemplate,
    items: [
      { name: 'nav.inspectionTemplates', href: '/inspection-templates', icon: ListChecks },
      { name: 'nav.configTemplates', href: '/config-templates', icon: FileText },
    ],
  },
  {
    name: 'nav.alertsAI',
    icon: AlertTriangle,
    items: [
      { name: 'nav.alerts', href: '/alerts', icon: Bell },
      { name: 'nav.alertRules', href: '/alert-rules', icon: ShieldCheck },
      { name: 'nav.alertMappings', href: '/alert-mappings', icon: Link2 },
      { name: 'nav.alertNoise', href: '/alert-noise', icon: Shield },
      { name: 'nav.alertCorrelation', href: '/alert-correlation-groups', icon: Layers },
      { name: 'nav.rootCauseAnalysis', href: '/root-cause-analysis', icon: Search },
      { name: 'nav.aiRootCause', href: '/ai-root-cause', icon: Brain },
      { name: 'nav.topology', href: '/topology', icon: Network },
      { name: 'nav.aiInsights', href: '/ai-insights', icon: Lightbulb },
      { name: 'nav.alertAutoAnalysis', href: '/alert-auto-analysis', icon: Zap },
      { name: 'nav.inspectionCenter', href: '/inspection-center', icon: Activity },
      { name: 'nav.alertProviders', href: '/alerts/providers', icon: Radio },
    ],
  },
  {
    name: 'nav.knowledgeReports',
    icon: BookMarked,
    items: [
      { name: 'nav.knowledge', href: '/knowledge', icon: BookOpen },
      { name: 'nav.auditLogs', href: '/audit', icon: Shield },
      { name: 'nav.notifications', href: '/notifications', icon: MessageSquare },
      { name: 'nav.reports', href: '/reports', icon: FileText },
    ],
  },
  {
    name: 'nav.systemUsers',
    icon: Cog,
    items: [
      { name: 'nav.users', href: '/users', icon: Users },
      { name: 'nav.metricTags', href: '/metric-tags', icon: Tag },
      { name: 'nav.frontendTests', href: '/frontend-tests', icon: FlaskConical },
      { name: 'nav.toolLinks', href: '/tool-links', icon: Link2 },
      { name: 'nav.settings', href: '/settings', icon: Settings },
    ],
  },
];
