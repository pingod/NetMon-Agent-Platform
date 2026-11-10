import { useState, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Send,
  Bot,
  User,
  Trash2,
  MessageSquare,
  Loader2,
  X,
  MinusCircle,
  AlertCircle,
  Maximize2,
  Minimize2,
  Sparkles,
  Server,
  Network,
  BookOpen,
  FileSpreadsheet,
  AlertTriangle,
  RotateCcw,
  Terminal,
  ShieldAlert,
  Activity,
  Database
} from 'lucide-react';
import api from '../../../lib/api';
import MarkdownOutput from '../../../shared/components/MarkdownOutput';
import { useToast } from '../../../contexts/ToastContext';
import { useTheme } from '../../../contexts/ThemeContext';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date | string;
}

interface Conversation {
  id: string;
  user_id?: string;
  messages: Message[];
  created_at?: Date | string;
  updated_at?: Date | string;
}

export default function ChatWidget() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { theme } = useTheme();
  const _isDark = theme === 'dark';
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [isMinimized, setIsMinimized] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [currentConversationId, setCurrentConversationId] = useState<string | null>(null);

  const { data: suggestions, error: suggestionsError } = useQuery({
    queryKey: ['copilot-suggestions'],
    queryFn: async () => {
      try {
        const res = await api.get('/copilot/suggestions');
        return res.data?.data || [];
      } catch {
        return [
          '去设备查询配置 (display current-configuration)',
          '多设备版本比对 (display version)',
          '查询设备接口状态 (display interface brief)',
          '查询服务器磁盘与负载 (df -h)',
          '下发配置测试 (安全只读防护验证)',
          '查询所有网络设备状态',
          '查看最新指标与采集数据',
          '列出交换机与服务器汇总表格',
          '展示当前网络拓扑结构与互联链路',
          'NetMon 系统使用指南与上手手册',
        ];
      }
    }
  });

  const { data: conversations, error: conversationsError } = useQuery({
    queryKey: ['copilot-conversations'],
    queryFn: async () => {
      try {
        const res = await api.get('/copilot/conversations');
        return res.data?.data || [];
      } catch {
        return [];
      }
    }
  });

  const isPasswordChangeRequired = (error: unknown) => {
    return (error as { response?: { status?: number } })?.response?.status === 403;
  };

  const currentConversation = conversations?.find((c: Conversation) => c.id === currentConversationId);

  const sendMessageMutation = useMutation({
    mutationFn: async ({ conversationId, message }: { conversationId?: string, message: string }) => {
      const res = await api.post('/copilot/chat', {
        conversationId,
        message
      });
      return res.data;
    },
    onMutate: async ({ conversationId, message }) => {
      await queryClient.cancelQueries({ queryKey: ['copilot-conversations'] });
      const previousConversations = queryClient.getQueryData<Conversation[]>(['copilot-conversations']);

      queryClient.setQueryData<Conversation[]>(['copilot-conversations'], (old) => {
        return old?.map((c) => {
          if (c.id === conversationId) {
            return {
              ...c,
              messages: [
                ...c.messages,
                { role: 'user', content: message, timestamp: new Date().toISOString() },
              ],
            };
          }
          return c;
        });
      });

      return { previousConversations };
    },
    onSuccess: (data) => {
      if (data?.data?.conversationId && !currentConversationId) {
        setCurrentConversationId(data.data.conversationId);
      }
      queryClient.invalidateQueries({ queryKey: ['copilot-conversations'] });
      setInputValue('');
    },
    onError: (_err, _variables, context) => {
      if (context?.previousConversations) {
        queryClient.setQueryData(['copilot-conversations'], context.previousConversations);
      }
      toast.error('发送消息失败，请重试');
    },
  });

  const createConversationMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/copilot/conversations');
      return res.data;
    },
    onSuccess: (data) => {
      if (data && (data.success || data.data?.id)) {
        const newId = data.data?.id;
        if (newId) {
          setCurrentConversationId(newId);
          queryClient.invalidateQueries({ queryKey: ['copilot-conversations'] });
        }
      } else {
        toast.error(`创建对话失败: ${data?.error || '服务暂不可用'}`);
      }
    },
    onError: (err: any) => {
      console.error('Failed to create conversation', err);
      toast.error('创建对话出错，请重试');
    },
  });

  const deleteConversationMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/copilot/conversations/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['copilot-conversations'] });
      setCurrentConversationId(null);
    },
  });

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [currentConversation?.messages, sendMessageMutation.isPending]);

  const handleSend = async (msg?: string) => {
    const message = msg || inputValue;
    if (!message.trim()) return;

    if (!currentConversationId) {
      try {
        const data = await createConversationMutation.mutateAsync();
        const convId = data?.data?.id;
        if (convId) {
          sendMessageMutation.mutate({ conversationId: convId, message });
        } else {
          sendMessageMutation.mutate({ message });
        }
      } catch {
        sendMessageMutation.mutate({ message });
      }
    } else {
      sendMessageMutation.mutate({ conversationId: currentConversationId, message });
    }
  };

  if (!isOpen) {
    return (
      <button
        onClick={() => setIsOpen(true)}
        className="fixed bottom-6 right-6 w-14 h-14 bg-gradient-to-r from-blue-600 via-indigo-600 to-purple-600 rounded-full shadow-lg shadow-blue-500/30 hover:shadow-2xl hover:shadow-blue-500/50 transition-all duration-300 flex items-center justify-center text-white hover:scale-110 active:scale-95 z-50 group"
        title="打开 NetMon 智能运维助手"
      >
        <Bot className="w-7 h-7 transition-transform group-hover:rotate-6" />
        <span className="absolute -top-1 -right-1 w-3.5 h-3.5 bg-emerald-500 border-2 border-slate-900 rounded-full animate-pulse" />
      </button>
    );
  }

  const bgMain = 'bg-surface';
  const borderColor = 'border-border';
  const sidebarBg = 'bg-background';
  const sidebarBorder = 'border-border';
  const chatBg = 'bg-background';
  const textPrimary = 'text-text-primary';
  const textSecondary = 'text-text-secondary';
  const textMuted = 'text-text-tertiary';
  const inputBg = 'bg-surface';
  const inputBorder = 'border-border';
  const cardBg = 'bg-surface';
  const cardBorder = 'border-border';
  const hoverBg = 'hover:bg-background';
  const dividerBg = 'border-border';
  const msgBg = 'bg-surface';
  const msgBorder = 'border-border';
  const minimizedBg = 'bg-surface';
  const minimizedBorder = 'border-border';

  // 快捷提问分组
  const quickActions = [
    { label: '一键巡检', icon: Activity, query: '巡检所有网络设备' },
    { label: '巡检核心', icon: Server, query: '帮我巡检核心交换机-1' },
    { label: '查数据库', icon: Database, query: '查一下数据库里有哪些表' },
    { label: '查告警表', icon: AlertTriangle, query: '查一下数据库里的告警' },
    { label: '查询配置', icon: Terminal, query: '去设备查询配置 (display current-configuration)' },
    { label: '版本比对', icon: Server, query: '多设备版本比对 (display version)' },
    { label: '接口状态', icon: Network, query: '查询设备接口状态 (display interface brief)' },
    { label: '设备状态', icon: Server, query: '查询所有网络设备状态' },
    { label: '汇总表格', icon: FileSpreadsheet, query: '列出交换机与服务器汇总表格' },
    { label: '网络拓扑', icon: Network, query: '展示当前网络拓扑结构与互联链路' },
  ];

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col items-end">
      {!isMinimized && (
        <div
          className={`${
            isExpanded
              ? 'w-[900px] max-w-[95vw] h-[720px] max-h-[90vh]'
              : 'w-[460px] max-w-[95vw] h-[620px] max-h-[85vh]'
          } ${bgMain} rounded-2xl shadow-2xl border ${borderColor} flex flex-col mb-3 overflow-hidden animate-slide-up transition-all duration-300 backdrop-blur-xl`}
        >
          {/* 顶栏 Header */}
          <div className="flex items-center justify-between px-4 py-3 bg-gradient-to-r from-blue-600 via-indigo-600 to-purple-600 flex-shrink-0 select-none shadow-md">
            <div className="flex items-center gap-2.5">
              <div className="w-7 h-7 rounded-lg bg-white/20 flex items-center justify-center backdrop-blur">
                <Bot className="w-4 h-4 text-white" />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold text-white text-sm">NetMon 智能运维助手</h3>
                  <span className="flex items-center gap-1 text-[10px] bg-emerald-500/20 text-emerald-200 border border-emerald-400/30 px-1.5 py-0.5 rounded-full font-medium">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                    AI 对话驱动
                  </span>
                </div>
                <p className="text-[11px] text-white/75 leading-none mt-0.5">
                  全栈监控 · 拓扑分析 · 采集指标 · 故障自愈
                </p>
              </div>
            </div>
            <div className="flex items-center gap-1">
              {/* 展开/还原按钮 */}
              <button
                onClick={() => setIsExpanded(!isExpanded)}
                className="p-1.5 hover:bg-white/20 text-white rounded-lg transition-all"
                title={isExpanded ? '还原标准窗口' : '宽屏展示模式（适合查看表格与拓扑）'}
              >
                {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
              </button>
              {/* 最小化按钮 */}
              <button
                onClick={() => setIsMinimized(true)}
                className="p-1.5 hover:bg-white/20 text-white rounded-lg transition-all"
                title="最小化"
              >
                <MinusCircle className="w-4 h-4" />
              </button>
              {/* 关闭按钮 */}
              <button
                onClick={() => setIsOpen(false)}
                className="p-1.5 hover:bg-white/20 text-white rounded-lg transition-all"
                title="关闭助手"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* 主体区 Body */}
          <div className="flex-1 flex overflow-hidden">
            {/* 会话侧边栏 */}
            <div className="w-36 bg-surface/50 border-r border-border flex flex-col flex-shrink-0">
              <button
                onClick={() => {
                  createConversationMutation.mutate();
                }}
                className="m-2 px-3 py-2 bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 disabled:opacity-50 text-white text-xs rounded-lg flex items-center justify-center gap-1.5 transition-all font-medium shadow-sm active:scale-95"
                disabled={createConversationMutation.isPending}
              >
                {createConversationMutation.isPending ? (
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                ) : (
                  <MessageSquare className="w-3.5 h-3.5" />
                )}
                {createConversationMutation.isPending ? '创建中...' : '新对话'}
              </button>

              <div className="flex-1 overflow-y-auto p-2 space-y-1 scrollbar-thin">
                {conversations?.map((c: Conversation) => {
                  const firstMsg = c.messages?.find((m) => m.role === 'user')?.content || '新对话';
                  const isSelected = c.id === currentConversationId;
                  return (
                    <div
                      key={c.id}
                      onClick={() => setCurrentConversationId(c.id)}
                      className={`group p-2 rounded-lg cursor-pointer transition-all text-xs border ${
                        isSelected
                          ? 'bg-blue-600/15 border-blue-500/40 text-blue-400 font-medium shadow-sm'
                          : `${cardBg} border-transparent ${textSecondary} ${hoverBg}`
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <span className="truncate flex-1 pr-1">{firstMsg}</span>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            deleteConversationMutation.mutate(c.id);
                          }}
                          className="opacity-0 group-hover:opacity-100 hover:text-red-400 transition-opacity p-0.5"
                          title="删除会话"
                        >
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </div>
                    </div>
                  );
                })}
                {(!conversations || conversations.length === 0) && (
                  <div className="text-center py-6 px-2 text-[11px] text-text-tertiary">
                    暂无历史会话
                  </div>
                )}
              </div>
            </div>

            {/* 对话消息展示区 */}
            <div className={`flex-1 flex flex-col ${chatBg} overflow-hidden`}>
              {/* 密码修改提示 */}
              {(isPasswordChangeRequired(suggestionsError) || isPasswordChangeRequired(conversationsError)) && (
                <div className="p-3 m-3 bg-yellow-500/10 border border-yellow-500/30 rounded-xl">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="w-4 h-4 text-yellow-400 flex-shrink-0 mt-0.5" />
                    <div>
                      <p className="text-yellow-300 font-medium text-xs">需要修改初始密码</p>
                      <p className="text-yellow-200/80 text-[11px] mt-0.5">
                        请前往设置页面更新密码后继续使用 NetMon 助手。
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {/* 欢迎与快速问答导航面板 */}
              {!currentConversationId ? (
                <div className="flex-1 overflow-y-auto p-6 flex flex-col items-center justify-center text-center scrollbar-thin">
                  <div className="w-14 h-14 bg-gradient-to-tr from-blue-600/20 to-purple-600/20 border border-blue-500/30 rounded-2xl flex items-center justify-center mb-3 shadow-inner">
                    <Bot className="w-8 h-8 text-blue-400 animate-pulse" />
                  </div>
                  <h3 className={`text-base font-semibold ${textPrimary} mb-1`}>
                    您好！我是 NetMon 智能运维助手
                  </h3>
                  <p className={`${textSecondary} text-xs max-w-md mb-5 leading-relaxed`}>
                    已接入全网监控、设备巡检调度、数据库安全检索与 AI 对话驱动系统。支持发指令巡检设备、访问数据库查数据与分析拓扑指标。
                  </p>

                  <div className="w-full max-w-lg grid grid-cols-2 gap-2 text-left">
                    {quickActions.map((action, idx) => {
                      const Icon = action.icon;
                      return (
                        <button
                          key={idx}
                          onClick={() => handleSend(action.query)}
                          className={`p-3 ${cardBg} ${hoverBg} border ${cardBorder} hover:border-blue-500/50 rounded-xl text-left text-xs ${textSecondary} hover:${textPrimary} transition-all duration-200 flex items-start gap-2.5 group shadow-sm hover:shadow`}
                        >
                          <div className="p-1.5 rounded-lg bg-blue-500/10 text-blue-400 group-hover:bg-blue-500 group-hover:text-white transition-colors mt-0.5">
                            <Icon className="w-4 h-4" />
                          </div>
                          <div>
                            <div className="font-semibold text-text-primary">{action.label}</div>
                            <div className="text-[11px] text-text-tertiary truncate max-w-[140px]">
                              {action.query}
                            </div>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                </div>
              ) : (
                <>
                  {/* 消息滚动区 */}
                  <div className="flex-1 overflow-y-auto p-4 space-y-4 scrollbar-thin">
                    {(currentConversation?.messages || []).map((msg: Message, index: number) => {
                      const isUser = msg.role === 'user';
                      return (
                        <div
                          key={`${msg.role}-${index}`}
                          className={`flex ${isUser ? 'justify-end' : 'justify-start'} animate-fade-in`}
                        >
                          <div className={`flex items-start gap-2.5 ${isUser ? 'max-w-[80%]' : 'max-w-[95%] w-full'}`}>
                            {!isUser && (
                              <div className="w-8 h-8 bg-gradient-to-r from-blue-600 to-indigo-600 rounded-xl flex items-center justify-center flex-shrink-0 shadow-md shadow-blue-500/20 mt-0.5">
                                <Bot className="w-4 h-4 text-white" />
                              </div>
                            )}
                            <div
                              className={`p-3.5 rounded-2xl text-xs leading-relaxed overflow-x-auto scrollbar-thin shadow-sm ${
                                isUser
                                  ? 'bg-gradient-to-r from-blue-600 to-indigo-600 text-white rounded-br-none shadow-blue-500/10'
                                  : `${msgBg} ${textSecondary} border ${msgBorder} rounded-tl-none w-full`
                              }`}
                            >
                              {!isUser && msg.content ? (
                                <MarkdownOutput content={msg.content} />
                              ) : (
                                <p className="text-sm whitespace-pre-wrap">{msg.content}</p>
                              )}
                              <div
                                className={`text-[10px] mt-1.5 opacity-60 ${
                                  isUser ? 'text-white text-right' : 'text-text-tertiary'
                                }`}
                              >
                                {typeof msg.timestamp === 'string'
                                  ? msg.timestamp.split('T')[1]?.substring(0, 5) || msg.timestamp.substring(11, 16) || ''
                                  : new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                              </div>
                            </div>
                            {isUser && (
                              <div className="w-8 h-8 bg-slate-700 border border-slate-600 rounded-xl flex items-center justify-center flex-shrink-0 shadow mt-0.5">
                                <User className="w-4 h-4 text-white" />
                              </div>
                            )}
                          </div>
                        </div>
                      );
                    })}

                    {sendMessageMutation.isPending && (
                      <div className="flex justify-start animate-fade-in">
                        <div className="flex items-start gap-2.5 max-w-[85%]">
                          <div className="w-8 h-8 bg-gradient-to-r from-blue-600 to-indigo-600 rounded-xl flex items-center justify-center flex-shrink-0 shadow-md shadow-blue-500/20">
                            <Bot className="w-4 h-4 text-white" />
                          </div>
                          <div className={`p-3.5 ${msgBg} ${textSecondary} rounded-2xl rounded-tl-none border ${msgBorder} flex items-center gap-3`}>
                            <Loader2 className="w-4 h-4 text-blue-400 animate-spin" />
                            <span className="text-xs text-text-tertiary">正在检索系统数据并生成回复...</span>
                          </div>
                        </div>
                      </div>
                    )}
                    <div ref={messagesEndRef} />
                  </div>

                  {/* 快捷指令芯片栏 */}
                  <div className="px-3 py-1.5 border-t border-border/60 bg-surface/40 flex items-center gap-1.5 overflow-x-auto scrollbar-none flex-shrink-0">
                    <span className="text-[10px] text-text-tertiary font-medium flex items-center gap-1 flex-shrink-0">
                      <Sparkles className="w-3 h-3 text-blue-400" />
                      快捷问答:
                    </span>
                    {quickActions.map((action, idx) => (
                      <button
                        key={idx}
                        onClick={() => handleSend(action.query)}
                        disabled={sendMessageMutation.isPending}
                        className="px-2.5 py-1 rounded-lg text-[11px] bg-surface hover:bg-blue-500/10 hover:text-blue-400 border border-border transition-colors whitespace-nowrap flex-shrink-0 disabled:opacity-50"
                      >
                        {action.label}
                      </button>
                    ))}
                  </div>

                  {/* 输入框区 Input bar */}
                  <div className={`p-3 border-t ${dividerBg} ${inputBg} flex-shrink-0`}>
                    <div className="flex gap-2">
                      <input
                        type="text"
                        value={inputValue}
                        onChange={(e) => setInputValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' && !e.shiftKey) {
                            e.preventDefault();
                            handleSend();
                          }
                        }}
                        placeholder="输入您的问题（如：查询交换机状态、展示网络拓扑、列出汇总表格...）"
                        className={`flex-1 ${inputBg} border ${inputBorder} rounded-xl px-3.5 py-2.5 text-xs ${textPrimary} placeholder:${textMuted} focus:outline-none focus:ring-2 focus:ring-blue-500/30 focus:border-blue-500 transition-all`}
                        disabled={sendMessageMutation.isPending}
                      />
                      <button
                        onClick={() => handleSend()}
                        disabled={!inputValue.trim() || sendMessageMutation.isPending}
                        className="px-4 py-2.5 bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed rounded-xl text-white transition-all shadow-md shadow-blue-500/20 flex items-center gap-1.5 font-medium text-xs active:scale-95"
                      >
                        {sendMessageMutation.isPending ? (
                          <Loader2 className="w-4 h-4 animate-spin" />
                        ) : (
                          <Send className="w-4 h-4" />
                        )}
                        <span>发送</span>
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 最小化状态徽标 */}
      {isMinimized && (
        <div className="flex gap-2 animate-bounce-subtle">
          <button
            onClick={() => setIsMinimized(false)}
            className={`w-12 h-12 ${minimizedBg} ${hoverBg} rounded-full shadow-xl border ${minimizedBorder} flex items-center justify-center text-blue-500 transition-all hover:scale-105 active:scale-95 group`}
            title="展开 NetMon 助手"
          >
            <MessageSquare className="w-5 h-5 group-hover:scale-110 transition-transform" />
          </button>
          <button
            onClick={() => setIsOpen(false)}
            className="w-12 h-12 bg-red-600 hover:bg-red-500 rounded-full shadow-xl shadow-red-500/20 flex items-center justify-center text-white transition-all hover:scale-105 active:scale-95"
            title="关闭"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
      )}
    </div>
  );
}
