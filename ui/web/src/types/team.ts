/** Team data types matching Go internal/store/team_store.go */

export interface TeamAccessSettings {
  allow_user_ids?: string[];
  deny_user_ids?: string[];
  allow_channels?: string[];
  deny_channels?: string[];
  progress_notifications?: boolean;
}

export interface TeamData {
  id: string;
  name: string;
  lead_agent_id: string;
  lead_agent_key?: string;
  description?: string;
  status: "active" | "archived";
  settings?: Record<string, unknown>;
  created_by: string;
  created_at?: string;
  updated_at?: string;
}

export interface TeamMemberData {
  team_id: string;
  agent_id: string;
  agent_key?: string;
  display_name?: string;
  frontmatter?: string;
  role: "lead" | "member" | "reviewer";
  joined_at?: string;
}

export interface TeamWorkspaceFile {
  id: string;
  team_id: string;
  channel: string;
  chat_id: string;
  file_name: string;
  mime_type?: string;
  size_bytes: number;
  uploaded_by: string;
  uploaded_by_key?: string;
  task_id?: string;
  pinned: boolean;
  tags?: string[];
  archived_at?: string;
  created_at?: string;
  updated_at?: string;
  missing?: boolean;
}

export interface TeamTaskData {
  id: string;
  team_id: string;
  subject: string;
  description?: string;
  status: "pending" | "in_progress" | "completed" | "blocked";
  owner_agent_id?: string;
  owner_agent_key?: string;
  blocked_by?: string[];
  priority: number;
  result?: string;
  created_at?: string;
  updated_at?: string;
}
