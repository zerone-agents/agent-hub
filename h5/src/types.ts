export type ActiveTab = 'chat' | 'agents' | 'knowledge' | 'profile';

export interface Agent {
  id: string;
  name: string;
  title: string;
  avatar: string;
  category: string;
  usageCount: string;
  description: string;
  capabilities: string;
  tags: string[];
  suggestedQuestions: string[];
  isTeam?: boolean;
  featured?: boolean;
}

export type DocumentType = 'xlsx' | 'html' | 'docx' | 'pdf' | 'md';

export interface KnowledgeFolder {
  id: string;
  name: string;
  description?: string;
  docCount?: number;
  chunkCount?: number;
  parseMethod?: string;
  category?: 'mine' | 'team';
  icon?: string;
  color?: string;
  createdAt: string;
  updatedAt: string;
  tags?: string[];
}

export interface KnowledgeDocument {
  id: string;
  name: string;
  type: DocumentType;
  category: 'recent' | 'mine' | 'team';
  folderId?: string;
  folderName?: string;
  size: string;
  updatedAt: string;
  content: string;
  summary?: string;
  tags?: string[];
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: string;
  agentId?: string;
  agentName?: string;
  agentAvatar?: string;
  isStreaming?: boolean;
}

export interface ScenarioPrompt {
  id: string;
  icon: string;
  title: string;
  prompt: string;
  category: string;
}
